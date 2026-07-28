//go:build !windows

package reviewtransaction

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func readPrivateDirectoryFiles(path string, limit int64, maxEntries int, aggregateLimit int64, afterOpen func()) ([]PrivateDirectoryFile, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || !privateIncidentPathSafe(before) {
		return nil, errUnsafePrivateDirectory
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parentFD, err := secureOpenLockParent(string(filepath.Separator), filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	defer unix.Close(parentFD)
	fd, err := unix.Openat(parentFD, filepath.Base(absolute), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(fd), path)
	defer directory.Close()
	openedDir, err := directory.Stat()
	if err != nil || !os.SameFile(before, openedDir) || !privateIncidentPathSafe(openedDir) {
		return nil, errPrivateDirectoryReplaced
	}
	if afterOpen != nil {
		afterOpen()
	}
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	if len(entries) > maxEntries {
		return nil, errors.New("private directory exceeds entry limit") // refusal:by-design world-action: bounded replay cannot choose audit entries to remove; the operator must archive excess files outside the private directory
	}
	files := make([]PrivateDirectoryFile, 0, len(entries))
	var aggregate int64
	for _, entry := range entries {
		if entry.Name() != filepath.Base(entry.Name()) {
			return nil, errUnsafePrivateDirectory
		}
		childFD, openErr := unix.Openat(fd, entry.Name(), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			return nil, errUnsafePrivateDirectory
		}
		file := os.NewFile(uintptr(childFD), entry.Name())
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || !privateIncidentPathSafe(info) || info.Size() < 1 || info.Size() > limit {
			_ = file.Close()
			return nil, errUnsafePrivateDirectory
		}
		aggregate += info.Size()
		if aggregate > aggregateLimit {
			_ = file.Close()
			return nil, errors.New("private directory exceeds aggregate byte limit") // refusal:by-design world-action: bounded replay cannot choose audit bytes to remove; the operator must archive excess files outside the private directory
		}
		payload, readErr := io.ReadAll(io.LimitReader(file, limit+1))
		after, afterErr := file.Stat()
		_ = file.Close()
		if readErr != nil || afterErr != nil || !os.SameFile(info, after) || after.Size() != int64(len(payload)) || len(payload) > int(limit) {
			return nil, errPrivateDirectoryReplaced
		}
		aggregate += int64(len(payload)) - info.Size()
		if aggregate > aggregateLimit {
			return nil, errors.New("private directory exceeds aggregate byte limit") // refusal:by-design world-action: bounded replay cannot choose audit bytes to remove; the operator must archive excess files outside the private directory
		}
		files = append(files, PrivateDirectoryFile{Name: entry.Name(), Payload: payload})
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(openedDir, current) {
		return nil, errPrivateDirectoryReplaced
	}
	return files, nil
}

func privateIncidentPathSafe(info os.FileInfo) bool {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
