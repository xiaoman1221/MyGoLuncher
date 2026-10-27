package app

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// TaskProgress is sent to the frontend as task:progress.
type TaskProgress struct {
	TaskID   string  `json:"taskId"`
	Title    string  `json:"title"`
	Stage    string  `json:"stage"`
	Message  string  `json:"message"`
	Current  int64   `json:"current"`
	Total    int64   `json:"total"`
	Percent  float64 `json:"percent"`
	Done     bool    `json:"done"`
	Failed   bool    `json:"failed"`
	Canceled bool    `json:"canceled"`
}

// TaskLogEntry is one timestamped line from a background task.
type TaskLogEntry struct {
	Seq      int64  `json:"seq"`
	Time     string `json:"time"`
	Stage    string `json:"stage"`
	Message  string `json:"message"`
	Current  int64  `json:"current"`
	Total    int64  `json:"total"`
	Done     bool   `json:"done"`
	Failed   bool   `json:"failed"`
	Canceled bool   `json:"canceled"`
}

// TaskLog is returned when a task is expanded in the task tray.
type TaskLog struct {
	TaskID   string         `json:"taskId"`
	Title    string         `json:"title"`
	Done     bool           `json:"done"`
	Failed   bool           `json:"failed"`
	Canceled bool           `json:"canceled"`
	Entries  []TaskLogEntry `json:"entries"`
}

// TaskLogEvent is emitted live as task:log.
type TaskLogEvent struct {
	TaskID string       `json:"taskId"`
	Entry  TaskLogEntry `json:"entry"`
}

type taskLog struct {
	TaskLog
	task      *progressTask
	nextSeq   int64
	updatedAt time.Time
}

type progressTask struct {
	app    *App
	id     string
	title  string
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	lastEmit  time.Time
	lastStage string
	finished  bool
	canceled  bool
}

var taskCounter atomic.Uint64

func (a *App) newTask(title string) *progressTask {
	id := fmt.Sprintf("task-%d-%d", time.Now().UnixMilli(), taskCounter.Add(1))
	ctx, cancel := context.WithCancel(a.context())
	t := &progressTask{app: a, id: id, title: title, ctx: ctx, cancel: cancel}
	a.taskMu.Lock()
	if a.taskLogs == nil {
		a.taskLogs = make(map[string]*taskLog)
	}
	now := time.Now()
	a.taskLogs[id] = &taskLog{
		TaskLog:   TaskLog{TaskID: id, Title: title, Entries: []TaskLogEntry{}},
		task:      t,
		updatedAt: now,
	}
	for key, entry := range a.taskLogs {
		if key != id && (len(a.taskLogs) > 80 || now.Sub(entry.updatedAt) > 2*time.Hour) {
			delete(a.taskLogs, key)
		}
	}
	a.taskMu.Unlock()
	t.emit("准备中", "正在准备…", 0, 0, false, false)
	return t
}

// CancelTask stops a running background task. Operations that accept the
// task context will abort their network/file work at the next checkpoint.
func (a *App) CancelTask(taskID string) error {
	a.taskMu.Lock()
	entry := a.taskLogs[taskID]
	var task *progressTask
	if entry != nil {
		task = entry.task
	}
	a.taskMu.Unlock()
	if task == nil {
		return fmt.Errorf("任务不存在或已结束")
	}
	return task.cancelTask()
}

// GetTaskLog returns the retained log for one background task.
func (a *App) GetTaskLog(taskID string) TaskLog {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	log, ok := a.taskLogs[taskID]
	if !ok {
		return TaskLog{TaskID: taskID, Entries: []TaskLogEntry{}}
	}
	result := log.TaskLog
	result.Entries = append([]TaskLogEntry(nil), log.Entries...)
	if result.Entries == nil {
		result.Entries = []TaskLogEntry{}
	}
	return result
}

func (t *progressTask) context() context.Context {
	if t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}

func (t *progressTask) appendLog(stage, message string, current, total int64, done, failed, canceled bool) {
	if t.app == nil {
		return
	}
	now := time.Now()
	t.app.taskMu.Lock()
	log := t.app.taskLogs[t.id]
	if log == nil {
		log = &taskLog{TaskLog: TaskLog{TaskID: t.id, Title: t.title, Entries: []TaskLogEntry{}}, task: t}
		t.app.taskLogs[t.id] = log
	}
	log.nextSeq++
	entry := TaskLogEntry{
		Seq: log.nextSeq, Time: now.Format("15:04:05"), Stage: stage, Message: message,
		Current: current, Total: total, Done: done, Failed: failed, Canceled: canceled,
	}
	log.Entries = append(log.Entries, entry)
	if len(log.Entries) > 600 {
		log.Entries = log.Entries[len(log.Entries)-500:]
	}
	log.Done = log.Done || done
	log.Failed = log.Failed || failed
	log.Canceled = log.Canceled || canceled
	log.updatedAt = now
	t.app.taskMu.Unlock()
	if t.app.ctx != nil {
		runtime.EventsEmit(t.app.ctx, "task:log", TaskLogEvent{TaskID: t.id, Entry: entry})
	}
}

func (t *progressTask) update(stage, message string, current, total int64) {
	t.emit(stage, message, current, total, false, false)
}

func (t *progressTask) cancelTask() error {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return fmt.Errorf("任务已结束")
	}
	t.finished = true
	t.canceled = true
	cancel := t.cancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	t.emitState("canceled", "任务已取消", 0, 0, true, false, true)
	return nil
}

func (t *progressTask) finish(err error) {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.finished = true
	cancel := t.cancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if err != nil {
		t.emit("error", err.Error(), 0, 0, true, true)
		return
	}
	t.emit("done", "操作完成", 1, 1, true, false)
}

func (t *progressTask) emit(stage, message string, current, total int64, done, failed bool) {
	t.emitState(stage, message, current, total, done, failed, false)
}

func (t *progressTask) emitState(stage, message string, current, total int64, done, failed, canceled bool) {
	t.mu.Lock()
	if t.finished && !done {
		t.mu.Unlock()
		return
	}
	now := time.Now()
	if !done && stage == t.lastStage && now.Sub(t.lastEmit) < 160*time.Millisecond {
		t.mu.Unlock()
		return
	}
	t.lastEmit = now
	t.lastStage = stage
	t.mu.Unlock()
	t.appendLog(stage, message, current, total, done, failed, canceled)
	percent := -1.0
	if total > 0 {
		percent = float64(current) / float64(total) * 100
		if percent > 100 {
			percent = 100
		}
	}
	if done && (failed || canceled) {
		percent = -1
	}
	if t.app.ctx == nil {
		return
	}
	runtime.EventsEmit(t.app.ctx, "task:progress", TaskProgress{
		TaskID:   t.id,
		Title:    t.title,
		Stage:    stage,
		Message:  message,
		Current:  current,
		Total:    total,
		Percent:  percent,
		Done:     done,
		Failed:   failed,
		Canceled: canceled,
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
