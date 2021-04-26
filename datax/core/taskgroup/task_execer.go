package taskgroup

import (
	"context"
	"fmt"
	"sync"

	"github.com/Breeze0806/go-etl/config"
	coreconst "github.com/Breeze0806/go-etl/datax/common/config/core"
	"github.com/Breeze0806/go-etl/datax/common/plugin/loader"
	"github.com/Breeze0806/go-etl/datax/common/spi/writer"
	"github.com/Breeze0806/go-etl/datax/core/statistics/communication"
	"github.com/Breeze0806/go-etl/datax/core/taskgroup/runner"
	"github.com/Breeze0806/go-etl/datax/core/transport/channel"
	"github.com/Breeze0806/go-etl/datax/core/transport/exchange"
	"go.uber.org/atomic"
)

type taskExecer struct {
	taskConf     *config.JSON //任务JSON配置
	taskID       int64        //任务编号
	ctx          context.Context
	channel      *channel.Channel //记录通道
	writerRunner runner.Runner    //写入运行器
	readerRunner runner.Runner    //执行运行器
	wg           sync.WaitGroup
	errors       chan error
	//todo: taskCommunication没用
	taskCommunication communication.Communication
	destroy           sync.Once
	key               string

	cancalMutex  sync.Mutex         //由于取消函数会被多线程调用,需要加锁
	cancel       context.CancelFunc //取消函数
	attemptCount *atomic.Int32      //执行次数
}

//newTaskExecer 根据上下文ctx，任务配置taskConf，前缀关键字prefixKey
//执行次数attemptCount生成任务执行器，当taskID不存在，工作器名字配置以及
//对应写入器和读取器不存在时会报错
func newTaskExecer(ctx context.Context, taskConf *config.JSON,
	jobID, taskGroupID int64, attemptCount int) (t *taskExecer, err error) {
	t = &taskExecer{
		taskConf:     taskConf,
		errors:       make(chan error, 2),
		ctx:          ctx,
		attemptCount: atomic.NewInt32(int32(attemptCount)),
	}
	t.channel, err = channel.NewChannel()
	if err != nil {
		return nil, err
	}

	t.taskID, err = taskConf.GetInt64(coreconst.TaskID)
	if err != nil {
		return nil, err
	}
	t.key = fmt.Sprintf("%v-%v-%v", jobID, taskGroupID, t.taskID)
	readName, writeName := "", ""
	readName, err = taskConf.GetString(coreconst.JobReaderName)
	if err != nil {
		return nil, err
	}

	writeName, err = taskConf.GetString(coreconst.JobWriterName)
	if err != nil {
		return nil, err
	}

	var readConf, writeConf *config.JSON
	readConf, err = taskConf.GetConfig(coreconst.JobReaderParameter)
	if err != nil {
		return nil, err
	}

	writeConf, err = taskConf.GetConfig(coreconst.JobWriterParameter)
	if err != nil {
		return nil, err
	}

	readTask, ok := loader.LoadReaderTask(readName)
	if !ok {
		return nil, fmt.Errorf("reader task name (%v) does not exist", readName)
	}
	readTask.SetJobID(jobID)
	readTask.SetTaskGroupID(taskGroupID)
	readTask.SetTaskID(t.taskID)
	readTask.SetPluginJobConf(readConf)
	readTask.SetPeerPluginName(writeName)
	readTask.SetPeerPluginJobConf(writeConf)
	exchanger := exchange.NewRecordExchangerWithoutTransformer(t.channel)
	t.readerRunner = runner.NewReader(readTask, exchanger, t.key)

	writeTask, ok := loader.LoadWriterTask(writeName)
	if !ok {
		return nil, fmt.Errorf("writer task name (%v) does not exist", writeName)
	}
	writeTask.SetJobID(jobID)
	writeTask.SetTaskGroupID(taskGroupID)
	writeTask.SetTaskID(t.taskID)
	writeTask.SetPluginJobConf(writeConf)
	writeTask.SetPeerPluginName(readName)
	writeTask.SetPeerPluginJobConf(readConf)
	t.writerRunner = runner.NewWriter(writeTask, exchanger, t.key)

	return
}

//Start 读取运行器和写入运行器分别在携程中执行
func (t *taskExecer) Start() {
	var ctx context.Context
	t.cancalMutex.Lock()
	ctx, t.cancel = context.WithCancel(t.ctx)
	t.cancalMutex.Unlock()
	log.Debugf("taskExecer %v start to run writer", t.key)
	t.wg.Add(1)
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer t.wg.Done()
		writerWg.Done()
		if err := t.writerRunner.Run(ctx); err != nil {
			t.errors <- fmt.Errorf("writer task(%v) fail, err: %v", t.Key(), err)
		}
	}()
	writerWg.Wait()
	log.Debugf("taskExecer %v start to run reader", t.key)
	var readerWg sync.WaitGroup
	t.wg.Add(1)
	readerWg.Add(1)
	go func() {
		defer t.wg.Done()
		readerWg.Done()
		if err := t.readerRunner.Run(ctx); err != nil {
			t.errors <- fmt.Errorf("reader task(%v) fail, err: %v", t.Key(), err)
		}
	}()
	readerWg.Wait()
}

//AttemptCount 执行次数
func (t *taskExecer) AttemptCount() int32 {
	return t.attemptCount.Load()
}

//Do 执行函数
func (t *taskExecer) Do() error {
	log.Debugf("taskExecer %v start to do", t.key)
	defer func() {
		t.attemptCount.Inc()
		log.Debugf("taskExecer %v end to do", t.key)
	}()
	//执行读取写入运行器
	t.Start()
	log.Debugf("taskExecer %v do wait runner stop", t.key)
	//等待读取写入运行器
	t.wg.Wait()
	var errs []error
	log.Debugf("taskExecer %v do wait runner err chan", t.key)
	//监听错误通道器获取错误
ErrorLoop:
	for {
		select {
		case err := <-t.errors:
			errs = append(errs, err)
		default:
			break ErrorLoop
		}
	}

	s := ""
	for i, v := range errs {
		if i > 0 {
			s += " "
		}
		s += v.Error()
	}
	if s != "" {
		return fmt.Errorf("%v", s)
	}
	return nil
}

//Key 关键之
func (t *taskExecer) Key() string {
	return t.key
}

//WriterSuportFailOverport 写入器是否支持错误重试
func (t *taskExecer) WriterSuportFailOverport() bool {
	task, ok := t.writerRunner.Plugin().(writer.Task)
	if !ok {
		return false
	}
	return task.SupportFailOver()
}

//Shutdown 通过cancel停止写入器，关闭reader和writer
func (t *taskExecer) Shutdown() {
	log.Debugf("taskExecer %v starts to shutdown", t.key)
	defer log.Debugf("taskExecer %v ends to shutdown", t.key)
	t.cancalMutex.Lock()
	if t.cancel != nil {
		t.cancel()
	}
	t.cancalMutex.Unlock()
	log.Debugf("taskExecer %v shutdown wait runner stop", t.key)
	t.wg.Wait()
	log.Debugf("taskExecer %v shutdown reader", t.key)
	t.readerRunner.Shutdown()

	log.Debugf("taskExecer %v shutdown writer", t.key)
	t.writerRunner.Shutdown()
}
