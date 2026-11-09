package fileio

import (
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

type renameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func moveNoReplace(source *os.Root, old string, target *os.Root, name string) error {
	return renameManaged(source, old, target, name, false)
}
func replaceManagedFile(source *os.Root, old string, target *os.Root, name string) error {
	return renameManaged(source, old, target, name, true)
}
func renameManaged(source *os.Root, old string, target *os.Root, name string, replace bool) error {
	// Use the directory handle for both opens and rename. A filesystem path
	// derived from Root.Name would lose Root's confinement under reparse races.
	dir, err := source.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	destination, err := target.Open(".")
	if err != nil {
		return err
	}
	defer destination.Close()
	objectName, err := windows.NewNTUnicodeString(old)
	if err != nil {
		return err
	}
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(dir.Fd()), ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE}
	var handle windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.DELETE|windows.SYNCHRONIZE, &attributes, &iosb, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN, windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	encoded, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}
	length := (len(encoded) - 1) * 2
	size := int(unsafe.Offsetof(renameInformation{}.FileName)) + length
	buffer := make([]byte, size)
	info := (*renameInformation)(unsafe.Pointer(&buffer[0]))
	if replace {
		info.ReplaceIfExists = 1
	}
	info.RootDirectory = windows.Handle(destination.Fd())
	info.FileNameLength = uint32(length)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&info.FileName[0])), length/2), encoded[:len(encoded)-1])
	err = windows.NtSetInformationFile(handle, &iosb, &buffer[0], uint32(size), windows.FileRenameInformation)
	runtime.KeepAlive(buffer)
	return err
}
