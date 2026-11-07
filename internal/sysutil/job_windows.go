//go:build windows

package sysutil

import (
	"errors"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Job owns launcher-spawned game processes. KILL_ON_JOB_CLOSE makes Windows
// terminate them even if the launcher exits unexpectedly or is force-closed.
type Job struct {
	mu     sync.Mutex
	handle windows.Handle
	closed bool
}

// NewKillOnCloseJob creates a job object that kills its processes when the
// launcher closes. A nil job means job objects are unavailable; callers should
// continue with explicit process tracking.
func NewKillOnCloseJob() *Job {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(handle)
		return nil
	}
	return &Job{handle: handle}
}

// Assign adds a started game process to the job.
func (j *Job) Assign(pid int) error {
	if j == nil || pid <= 0 {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || j.handle == 0 {
		return errors.New("游戏清理任务已关闭")
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(j.handle, process)
}

// Close releases the job. Because KILL_ON_JOB_CLOSE is set, any process still
// assigned to it is terminated by Windows when the handle is closed.
func (j *Job) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || j.handle == 0 {
		return nil
	}
	j.closed = true
	err := windows.CloseHandle(j.handle)
	j.handle = 0
	return err
}
