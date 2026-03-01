//go:build darwin || linux

package main

import (
	"syscall"
)

func getDiskSpace(root string) (freeBytes, totalBytes int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return 0, 0, err
	}
	total := int64(stat.Blocks) * int64(stat.Bsize)
	free := int64(stat.Bavail) * int64(stat.Bsize) // Bavail: space available to non-root
	return free, total, nil
}
