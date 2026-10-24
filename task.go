package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// TaskProgress is sent to the frontend as task:progress.
type TaskProgress struct {
	TaskID  string  `json:"taskId"`
	Title   string  `json:"title"`
	Stage   string  `json:"stage"`
	Message string  `json:"message"`
	Current int64   `json:"current"`
	Total   int64   `json:"total"`
	Percent float64 `json:"percent"`
	Done    bool    `json:"done"`
	Failed  bool    `json:"failed"`
}

type progressTask struct {
	app   *App
	id    string
	title string

	mu        sync.Mutex
	lastEmit  time.Time
	lastStage string
	finished  bool
}

var taskCounter atomic.Uint64

func (a *App) newTask(title string) *progressTask {
	id := fmt.Sprintf("task-%d-%d", time.Now().UnixMilli(), taskCounter.Add(1))
	t := &progressTask{app: a, id: id, title: title}
	t.emit("准备中", "正在准备…", 0, 0, false, false)
	return t
}

func (t *progressTask) update(stage, message string, current, total int64) {
	t.emit(stage, message, current, total, false, false)
}

func (t *progressTask) finish(err error) {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.finished = true
	t.mu.Unlock()
	if err != nil {
		t.emit("error", err.Error(), 0, 0, true, true)
		return
	}
	t.emit("done", "操作完成", 1, 1, true, false)
}

func (t *progressTask) emit(stage, message string, current, total int64, done, failed bool) {
	t.mu.Lock()
	now := time.Now()
	if !done && stage == t.lastStage && now.Sub(t.lastEmit) < 160*time.Millisecond {
		t.mu.Unlock()
		return
	}
	t.lastEmit = now
	t.lastStage = stage
	t.mu.Unlock()
	percent := -1.0
	if total > 0 {
		percent = float64(current) / float64(total) * 100
		if percent > 100 {
			percent = 100
		}
	}
	if done && failed {
		percent = -1
	}
	if t.app.ctx == nil {
		return
	}
	runtime.EventsEmit(t.app.ctx, "task:progress", TaskProgress{
		TaskID:  t.id,
		Title:   t.title,
		Stage:   stage,
		Message: message,
		Current: current,
		Total:   total,
		Percent: percent,
		Done:    done,
		Failed:  failed,
	})
}

func (t *progressTask) mojangProgress(stagePrefix string) func(string, string, int, int) {
	return func(stage, message string, current, total int) {
		if stagePrefix != "" {
			stage = stagePrefix + ":" + stage
		}
		t.update(stage, message, int64(current), int64(total))
	}
}

func (t *progressTask) javaBytesProgress(stagePrefix string) func(string, string, int64, int64) {
	return func(stage, message string, current, total int64) {
		if stagePrefix != "" {
			stage = stagePrefix + ":" + stage
		}
		t.update(stage, message, current, total)
	}
}
