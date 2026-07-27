//go:build windows

package reviewtransaction

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
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
		file, openErr := openWindowsRARChild(directory, entry.Name())
		if openErr != nil {
			return nil, errUnsafeRARAuthorityPath
		}
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

func openWindowsRARChild(directory *os.File, name string) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), ObjectName: objectName,
		RootDirectory: windows.Handle(directory.Fd()), Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ|windows.READ_CONTROL, attributes, &status, nil,
		windows.FILE_ATTRIBUTE_NORMAL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if openWindowsRARFileUnsafe(file) {
		_ = file.Close()
		return nil, errUnsafeRARAuthorityPath
	}
	return file, nil
}

func rarPathUnsafe(path string, info fs.FileInfo) bool {
	if path == "" || info == nil || info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attributes, err := windows.GetFileAttributes(name)
	return err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func createPrivateRARDirectory(path string) (bool, error) {
	descriptor, err := ownerOnlyRARSecurityDescriptor(true)
	if err != nil {
		return false, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attributes := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	err = windows.CreateDirectory(name, &attributes)
	runtime.KeepAlive(descriptor)
	created := err == nil
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
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
	descriptor, err := ownerOnlyRARSecurityDescriptor(false)
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attributes := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		&attributes,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	opened, statErr := file.Stat()
	current, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(opened, current) || rarPathUnsafe(path, current) ||
		!privateOpenRARPathSafe(file, opened) {
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

func privateRARPathSafe(path string, info fs.FileInfo) bool {
	if path == "" || info == nil || rarPathUnsafe(path, info) {
		return false
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	return err == nil && privateRARSecurityDescriptorSafe(descriptor, info.IsDir())
}

func privateOpenRARPathSafe(file *os.File, info fs.FileInfo) bool {
	if file == nil || info == nil || openWindowsRARFileUnsafe(file) {
		return false
	}
	descriptor, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()),
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	return err == nil && privateRARSecurityDescriptorSafe(descriptor, info.IsDir())
}

func rarRepositoryDirectorySafe(path string, info fs.FileInfo) bool {
	if path == "" || info == nil || !info.IsDir() || rarPathUnsafe(path, info) {
		return false
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		return false
	}
	return rarSharedSecurityDescriptorOwnedByCurrentProcess(descriptor)
}

func rarRepositoryOpenDirectorySafe(file *os.File, info fs.FileInfo) bool {
	if file == nil || info == nil || !info.IsDir() || openWindowsRARFileUnsafe(file) {
		return false
	}
	descriptor, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()),
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		return false
	}
	return rarSharedSecurityDescriptorOwnedByCurrentProcess(descriptor)
}

func openRARPathNoFollow(path string, directory bool) (*os.File, error) {
	handle, err := openWindowsRARObject(ntPath(path), directory)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if openWindowsRARFileUnsafe(file) {
		_ = file.Close()
		return nil, errUnsafeRARAuthorityPath
	}
	return file, nil
}

func openWindowsRARObject(objectPath string, directory bool) (windows.Handle, error) {
	open := func(path string) (windows.Handle, error) {
		objectName, err := windows.NewNTUnicodeString(path)
		if err != nil {
			return 0, err
		}
		attributes := &windows.OBJECT_ATTRIBUTES{
			Length:     uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
			ObjectName: objectName,
			Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
		}
		options := uint32(windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_OPEN_REPARSE_POINT)
		if directory {
			options |= windows.FILE_DIRECTORY_FILE
		} else {
			options |= windows.FILE_NON_DIRECTORY_FILE
		}
		var handle windows.Handle
		var status windows.IO_STATUS_BLOCK
		err = windows.NtCreateFile(
			&handle,
			windows.FILE_GENERIC_READ|windows.READ_CONTROL,
			attributes,
			&status,
			nil,
			windows.FILE_ATTRIBUTE_NORMAL,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			windows.FILE_OPEN,
			options,
			0,
			0,
		)
		return handle, err
	}
	handle, err := open(objectPath)
	if !errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED) {
		return handle, err
	}
	directPath, resolveErr := directLocalDriveObjectPath(objectPath, queryWindowsDosDevice)
	if resolveErr != nil {
		return 0, fmt.Errorf("resolve secure Windows RAR path after %w: %v", err, resolveErr)
	}
	return open(directPath)
}

func openWindowsRARFileUnsafe(file *os.File) bool {
	if file == nil {
		return true
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return true
	}
	return info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func privateRARSecurityDescriptorSafe(
	descriptor *windows.SECURITY_DESCRIPTOR,
	directory bool,
) bool {
	if descriptor == nil || !descriptor.IsValid() {
		return false
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PRESENT == 0 ||
		control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	if !rarSecurityDescriptorOwnedByCurrentUser(descriptor) {
		return false
	}
	currentUser, err := currentRARWindowsUserSID()
	if err != nil {
		return false
	}
	dacl, defaulted, err := descriptor.DACL()
	if err != nil || dacl == nil || defaulted || dacl.AceCount != 1 {
		return false
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil || ace == nil ||
		ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		return false
	}
	wantFlags := uint8(0)
	if directory {
		wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	if ace.Header.AceFlags != wantFlags || !ownerOnlyRARWindowsAccessMask(ace.Mask) {
		return false
	}
	const sidOffset = unsafe.Offsetof(windows.ACCESS_ALLOWED_ACE{}.SidStart)
	if uintptr(ace.Header.AceSize) < sidOffset+unsafe.Sizeof(ace.SidStart) {
		return false
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	return aceSID.IsValid() &&
		uintptr(ace.Header.AceSize) >= sidOffset+uintptr(aceSID.Len()) &&
		aceSID.Equals(currentUser)
}

func rarSecurityDescriptorOwnedByCurrentUser(
	descriptor *windows.SECURITY_DESCRIPTOR,
) bool {
	if descriptor == nil || !descriptor.IsValid() {
		return false
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return false
	}
	currentUser, err := currentRARWindowsUserSID()
	return err == nil && owner.Equals(currentUser)
}

func rarSharedSecurityDescriptorOwnedByCurrentProcess(
	descriptor *windows.SECURITY_DESCRIPTOR,
) bool {
	if descriptor == nil || !descriptor.IsValid() {
		return false
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return false
	}
	currentUser, err := currentRARWindowsUserSID()
	if err == nil && owner.Equals(currentUser) {
		return true
	}
	tokenOwner, err := currentRARWindowsTokenOwnerSID()
	return err == nil && owner.Equals(tokenOwner)
}

func ownerOnlyRARSecurityDescriptor(directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	currentUser, err := currentRARWindowsUserSID()
	if err != nil {
		return nil, err
	}
	sid := currentUser.String()
	if sid == "" {
		return nil, errors.New("current Windows user SID is unavailable")
	}
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + sid + "D:P(A;" + inheritance + ";GA;;;" + sid + ")",
	)
	if err != nil || descriptor == nil || !descriptor.IsValid() {
		if err != nil {
			return nil, fmt.Errorf("build owner-only RAR DACL: %w", err)
		}
		return nil, errors.New("owner-only RAR DACL is invalid")
	}
	return descriptor, nil
}

func currentRARWindowsUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !user.User.Sid.IsValid() {
		if err != nil {
			return nil, fmt.Errorf("resolve current Windows user SID: %w", err)
		}
		return nil, errors.New("current Windows user SID is invalid")
	}
	sid, err := user.User.Sid.Copy()
	if err != nil {
		return nil, fmt.Errorf("copy current Windows user SID: %w", err)
	}
	return sid, nil
}

type rarWindowsTokenOwner struct {
	Owner *windows.SID
}

func currentRARWindowsTokenOwnerSID() (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()
	var size uint32
	err := windows.GetTokenInformation(
		token,
		windows.TokenOwner,
		nil,
		0,
		&size,
	)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) ||
		size < uint32(unsafe.Sizeof(rarWindowsTokenOwner{})) {
		if err != nil {
			return nil, fmt.Errorf(
				"resolve current Windows token owner size: %w",
				err,
			)
		}
		return nil, errors.New(
			"current Windows token owner has an invalid size",
		)
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(
		token,
		windows.TokenOwner,
		&buffer[0],
		size,
		&size,
	); err != nil {
		return nil, fmt.Errorf(
			"resolve current Windows token owner: %w",
			err,
		)
	}
	value := (*rarWindowsTokenOwner)(unsafe.Pointer(&buffer[0]))
	if value.Owner == nil || !value.Owner.IsValid() {
		return nil, errors.New("current Windows token owner SID is invalid")
	}
	owner, err := value.Owner.Copy()
	runtime.KeepAlive(buffer)
	if err != nil {
		return nil, fmt.Errorf("copy current Windows token owner SID: %w", err)
	}
	return owner, nil
}

func ownerOnlyRARWindowsAccessMask(mask windows.ACCESS_MASK) bool {
	const fileAllAccess windows.ACCESS_MASK = windows.STANDARD_RIGHTS_REQUIRED |
		windows.SYNCHRONIZE | windows.ACCESS_MASK(0x1ff)
	return mask == windows.GENERIC_ALL || mask == fileAllAccess
}
