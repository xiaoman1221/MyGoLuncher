package app

import (
	"context"
	"testing"
)

func TestTaskCancel(t *testing.T) {
	app := &App{}
	task := app.newTask("取消测试")
	if err := app.CancelTask(task.id); err != nil {
		t.Fatal(err)
	}
	log := app.GetTaskLog(task.id)
	if !log.Done || !log.Canceled || log.Failed {
		t.Fatalf("unexpected canceled log: %#v", log)
	}
	if task.context().Err() != context.Canceled {
		t.Fatalf("task context was not canceled: %v", task.context().Err())
	}
	if err := app.CancelTask(task.id); err == nil {
		t.Fatal("expected second cancel to fail")
	}
}
