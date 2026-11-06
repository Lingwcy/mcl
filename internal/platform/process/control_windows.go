package process

import (
	"errors"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type job struct{ handle windows.Handle }

func configure(cmd *exec.Cmd) {
	// Attach to the Job before allowing Java to create descendants.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW}
}

func attach(cmd *exec.Cmd, beforeResume func() error) (controller, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			windows.TerminateJobObject(handle, 1)
			windows.CloseHandle(handle)
		}
	}()
	// Owner death during startup must not strand a suspended child. commit
	// clears this flag only after registration and thread resume.
	if err := jobLimits(handle, windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE); err != nil {
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(handle, process); err != nil {
		return nil, err
	}
	if err := beforeResume(); err != nil {
		return nil, err
	}
	// os/exec owns/closes the primary thread handle. Enumerate threads while
	// the process is still suspended, then resume them only after assignment.
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	var threads []uint32
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID == uint32(cmd.Process.Pid) {
			threads = append(threads, entry.ThreadID)
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	if len(threads) == 0 {
		return nil, errors.New("suspended Java process has no primary thread")
	}
	for _, id := range threads {
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, id)
		if err != nil {
			return nil, err
		}
		_, err = windows.ResumeThread(thread)
		windows.CloseHandle(thread)
		if err != nil {
			windows.TerminateJobObject(handle, 1)
			return nil, err
		}
	}
	ok = true
	return job{handle: handle}, nil
}

func (j job) terminate() error { return windows.TerminateJobObject(j.handle, 1) }
func (j job) kill() error      { return windows.TerminateJobObject(j.handle, 1) }
func (j job) close()           { windows.CloseHandle(j.handle) }
func (j job) commit() error    { return jobLimits(j.handle, 0) }

func jobLimits(handle windows.Handle, flags uint32) error {
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = flags
	_, err := windows.SetInformationJobObject(handle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	return err
}
