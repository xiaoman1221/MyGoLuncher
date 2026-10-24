//go:build windows

package sysinfo

import (
	"syscall"
	"unsafe"
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var globalMemoryStatusEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// TotalMemoryMB returns installed physical memory.
func TotalMemoryMB() int {
	status := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	ok, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 || status.TotalPhys == 0 {
		return 0
	}
	return int(status.TotalPhys / (1024 * 1024))
}
