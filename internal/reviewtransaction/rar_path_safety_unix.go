//go:build !windows

package reviewtransaction

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func readPrivateDirectoryFiles(path string, limit int64, maxEntries int, aggregateLimit int64, afterOpen func()) ([]PrivateDirectoryFile, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || !privateRARPathSafe(path, before) {
		return nil, errUnsafeRARAuthorityPath
	}
	directory, err := openRARPathNoFollow(path, true)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	openedDir, err := directory.Stat()
	if err != nil || !os.SameFile(before, openedDir) || !privateOpenRARPathSafe(directory, openedDir) {
		return nil, errRARAuthorityPathReplaced
	}
	if afterOpen != nil {
		afterOpen()
	}
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	if len(entries) > maxEntries {
		return nil, errors.New("private directory exceeds entry limit")
	}
	files := make([]PrivateDirectoryFile, 0, len(entries))
	var aggregate int64
	for _, entry := range entries {
		if entry.Name() != filepath.Base(entry.Name()) {
			return nil, errUnsafeRARAuthorityPath
		}
		fd, openErr := unix.Openat(int(directory.Fd()), entry.Name(), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			return nil, errUnsafeRARAuthorityPath
		}
		file := os.NewFile(uintptr(fd), entry.Name())
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || !privateOpenRARPathSafe(file, info) || info.Size() < 1 || info.Size() > limit {
			_ = file.Close()
			return nil, errUnsafeRARAuthorityPath
		}
		aggregate += info.Size()
		if aggregate > aggregateLimit {
			_ = file.Close()
			return nil, errors.New("private directory exceeds aggregate byte limit")
		}
		payload, readErr := io.ReadAll(io.LimitReader(file, limit+1))
		after, afterErr := file.Stat()
		_ = file.Close()
		if readErr != nil || afterErr != nil || !os.SameFile(info, after) || after.Size() != int64(len(payload)) || len(payload) > int(limit) {
			return nil, errRARAuthorityPathReplaced
		}
		aggregate += int64(len(payload)) - info.Size()
		if aggregate > aggregateLimit {
			return nil, errors.New("private directory exceeds aggregate byte limit")
		}
		files = append(files, PrivateDirectoryFile{Name: entry.Name(), Payload: payload})
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(openedDir, current) {
		return nil, errRARAuthorityPathReplaced
	}
	return files, nil
}

func rarPathUnsafe(_ string, info fs.FileInfo) bool {
	return info == nil || info.Mode()&os.ModeSymlink != 0
}

func createPrivateRARDirectory(path string) (bool, error) {
	err := os.Mkdir(path, 0o700)
	created := err == nil
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return false, err
	}
	if err := validatePrivateRARDirectory(path); err != nil {
		if created {
			_ = os.Remove(path)
		}
		return false, err
	}
	return created, nil
}

func createPrivateRARFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	opened, statErr := file.Stat()
	current, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(opened, current) || !privateOpenRARPathSafe(file, opened) {
		_ = file.Close()
		_ = os.Remove(path)
		if statErr != nil {
			return nil, statErr
		}
		if pathErr != nil {
			return nil, pathErr
		}
		return nil, errUnsafeRARAuthorityPath
	}
	return file, nil
}

func privateRARPathSafe(_ string, info fs.FileInfo) bool {
	if info == nil || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func privateOpenRARPathSafe(_ *os.File, info fs.FileInfo) bool {
	return privateRARPathSafe("", info)
}

func rarRepositoryDirectorySafe(_ string, info fs.FileInfo) bool {
	if info == nil || !info.IsDir() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func rarRepositoryOpenDirectorySafe(_ *os.File, info fs.FileInfo) bool {
	return rarRepositoryDirectorySafe("", info)
}

func openRARPathNoFollow(path string, directory bool) (*os.File, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// RAR path safety is a distinct security boundary from the store lock
	// walk; it keeps the unconditional root-anchored walk verbatim by always
	// passing the filesystem root as the anchor.
	parentFD, err := secureOpenLockParent(string(filepath.Separator), filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	defer unix.Close(parentFD)
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(parentFD, filepath.Base(absolute), flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
