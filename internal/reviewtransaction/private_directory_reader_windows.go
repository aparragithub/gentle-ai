//go:build windows

package reviewtransaction

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func readPrivateDirectoryFiles(path string, limit int64, maxEntries int, aggregateLimit int64, afterOpen func()) ([]PrivateDirectoryFile, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() {
		return nil, errUnsafePrivateDirectory
	}
	directory, err := openWindowsPrivateObject(path, 0, true)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	openedDir, err := directory.Stat()
	if err != nil || !os.SameFile(before, openedDir) || windowsPrivateFileUnsafe(directory) {
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
		return nil, errors.New("private directory exceeds entry limit")
	}
	files := make([]PrivateDirectoryFile, 0, len(entries))
	var aggregate int64
	for _, entry := range entries {
		if entry.Name() != filepath.Base(entry.Name()) {
			return nil, errUnsafePrivateDirectory
		}
		file, openErr := openWindowsPrivateObject(entry.Name(), windows.Handle(directory.Fd()), false)
		if openErr != nil {
			return nil, errUnsafePrivateDirectory
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || windowsPrivateFileUnsafe(file) || info.Size() < 1 || info.Size() > limit {
			_ = file.Close()
			return nil, errUnsafePrivateDirectory
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
			return nil, errPrivateDirectoryReplaced
		}
		aggregate += int64(len(payload)) - info.Size()
		if aggregate > aggregateLimit {
			return nil, errors.New("private directory exceeds aggregate byte limit")
		}
		files = append(files, PrivateDirectoryFile{Name: entry.Name(), Payload: payload})
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(openedDir, current) {
		return nil, errPrivateDirectoryReplaced
	}
	return files, nil
}

func openWindowsPrivateObject(path string, root windows.Handle, directory bool) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(path)
	if err != nil {
		return nil, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), ObjectName: objectName,
		RootDirectory: root, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	options := uint32(windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_OPEN_REPARSE_POINT)
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
	} else {
		options |= windows.FILE_NON_DIRECTORY_FILE
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ, attributes, &status, nil,
		windows.FILE_ATTRIBUTE_NORMAL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, options, 0, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if windowsPrivateFileUnsafe(file) {
		_ = file.Close()
		return nil, errUnsafePrivateDirectory
	}
	return file, nil
}

func windowsPrivateFileUnsafe(file *os.File) bool {
	if file == nil {
		return true
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return true
	}
	return info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
