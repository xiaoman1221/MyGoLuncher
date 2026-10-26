//go:build !windows

package sysutil

// Job is a no-op on platforms without Windows job objects.
type Job struct{}

func NewKillOnCloseJob() *Job { return &Job{} }

func (j *Job) Assign(pid int) error { return nil }

func (j *Job) Close() error { return nil }
