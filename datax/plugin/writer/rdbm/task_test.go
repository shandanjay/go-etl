package rdbm

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Breeze0806/go-etl/config"
)

func TestTask_Init(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		t       *Task
		args    args
		conf    *config.JSON
		jobConf *config.JSON
		wantErr bool
	}{
		{
			name: "1",
			t: NewTask(newMockDbHandler(func(name string, conf *config.JSON) (Execer, error) {
				return &MockExecer{}, nil
			})),
			args: args{
				ctx: context.TODO(),
			},
			conf:    TestJSONFromFile(filepath.Join("resources", "plugin.json")),
			jobConf: TestJSONFromString(`{}`),
		},
		{
			name: "2",
			t: NewTask(newMockDbHandler(func(name string, conf *config.JSON) (Execer, error) {
				return &MockExecer{}, nil
			})),
			args: args{
				ctx: context.TODO(),
			},
			conf:    TestJSONFromString(`{}`),
			jobConf: TestJSONFromString(`{}`),
			wantErr: true,
		},
		{
			name: "3",
			t: NewTask(newMockDbHandler(func(name string, conf *config.JSON) (Execer, error) {
				return &MockExecer{}, nil
			})),
			args: args{
				ctx: context.TODO(),
			},
			conf: TestJSONFromFile(filepath.Join("resources", "plugin.json")),
			jobConf: TestJSONFromString(`{
				"username": 1		
			}`),
			wantErr: true,
		},
		{
			name: "4",
			t: NewTask(newMockDbHandler(func(name string, conf *config.JSON) (Execer, error) {
				return nil, errors.New("mock error")
			})),
			args: args{
				ctx: context.TODO(),
			},
			conf:    TestJSONFromFile(filepath.Join("resources", "plugin.json")),
			jobConf: TestJSONFromString(`{}`),
			wantErr: true,
		},
		{
			name: "5",
			t: NewTask(newMockDbHandler(func(name string, conf *config.JSON) (Execer, error) {
				return &MockExecer{
					PingErr: errors.New("mock error"),
				}, nil
			})),
			args: args{
				ctx: context.TODO(),
			},
			conf:    TestJSONFromFile(filepath.Join("resources", "plugin.json")),
			jobConf: TestJSONFromString(`{}`),
			wantErr: true,
		},
		{
			name: "6",
			t: NewTask(newMockDbHandler(func(name string, conf *config.JSON) (Execer, error) {
				return &MockExecer{
					FetchErr: errors.New("mock error"),
				}, nil
			})),
			args: args{
				ctx: context.TODO(),
			},
			conf:    TestJSONFromFile(filepath.Join("resources", "plugin.json")),
			jobConf: TestJSONFromString(`{}`),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.t.SetPluginConf(tt.conf)
			tt.t.SetPluginJobConf(tt.jobConf)
			if err := tt.t.Init(tt.args.ctx); (err != nil) != tt.wantErr {
				t.Errorf("Task.Init() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTask_Destroy(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		t       *Task
		args    args
		wantErr bool
	}{
		{
			name: "1",
			t: &Task{
				Execer: &MockExecer{},
			},
			args: args{
				ctx: context.TODO(),
			},
		},
		{
			name: "2",
			t: &Task{
				Execer: nil,
			},
			args: args{
				ctx: context.TODO(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.t.Destroy(tt.args.ctx); (err != nil) != tt.wantErr {
				t.Errorf("Task.Destroy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}