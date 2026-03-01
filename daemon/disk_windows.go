//go:build windows

package main

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceExW = modkernel32.NewProc("GetDiskFreeSpaceExW")
)

func getDiskSpace(root string) (freeBytes, totalBytes int64, err error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return 0, 0, err
	}
	pathPtr, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return 0, 0, err
	}
	var free, total, totalFree int64
	ret, _, sysErr := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&free)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if ret == 0 {
		if sysErr != nil {
			return 0, 0, sysErr
		}
		return 0, 0, syscall.EINVAL
	}
	return free, total, nil
}
