package main

import (
	"testing"
)

func TestTaskLogRetainsEntries(t *testing.T) {
	app := &App{}
	task := app.newTask("测试任务")
	task.update("download", "正在下载文件", 5, 10)
	task.finish(nil)
	log := app.GetTaskLog(task.id)
	if log.TaskID != task.id || !log.Done || log.Failed {
		t.Fatalf("unexpected task log: %#v", log)
	}
	if len(log.Entries) < 3 {
		t.Fatalf("expected initial, progress and done entries, got %d", len(log.Entries))
	}
	last := log.Entries[len(log.Entries)-1]
	if !last.Done || last.Message != "操作完成" {
		t.Fatalf("unexpected final entry: %#v", last)
	}
	if got := app.GetTaskLog("missing"); len(got.Entries) != 0 {
		t.Fatalf("missing task returned entries: %#v", got)
	}
}
