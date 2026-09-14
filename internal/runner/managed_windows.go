//go:build windows

package runner

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

// Slice B2.2 (mutate-staged-v1.9): Windows tree ownership through a Job
// Object.
//
// The job carries JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, so closing its last
// handle ends the leader and every descendant even when dharness itself dies
// without running cleanup. The leader starts CREATE_SUSPENDED, is assigned
// to the job, and only then is resumed through NtResumeProcess (os/exec
// exposes no thread handle, so ResumeThread is unreachable): no descendant
// can run outside the job. Measured in H-TK: the shim, node, and proxy
// workers are gone within milliseconds of the job closing, including the
// owner-crash control. There is no taskkill fallback anywhere in this path.

const (
	createSuspendedFlag             = 0x00000004
	jobLimitKillOnJobClose          = 0x00002000
	jobInfoExtendedLimitInformation = 9
	processAssignAccess             = 0x1F0FFF // PROCESS_ALL_ACCESS: assign needs SET_QUOTA|TERMINATE and resume needs SUSPEND_RESUME; measured working in H-TK
)

var (
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")
	ntdllDLL    = syscall.NewLazyDLL("ntdll.dll")

	procCreateJobObjectW   = kernel32DLL.NewProc("CreateJobObjectW")
	procSetInformationJob  = kernel32DLL.NewProc("SetInformationJobObject")
	procAssignProcessToJob = kernel32DLL.NewProc("AssignProcessToJobObject")
	procOpenProcessHandle  = kernel32DLL.NewProc("OpenProcess")
	procNtResumeProcess    = ntdllDLL.NewProc("NtResumeProcess")
	procCloseHandleObject  = kernel32DLL.NewProc("CloseHandle")
)

type jobBasicLimits struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobIOCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobExtendedLimits struct {
	Basic    jobBasicLimits
	IO       jobIOCounters
	ProcMem  uintptr
	JobMem   uintptr
	PeakProc uintptr
	PeakJob  uintptr
}

func createKillOnCloseJob() (syscall.Handle, error) {
	handle, _, err := procCreateJobObjectW.Call(0, 0)
	if handle == 0 {
		return 0, fmt.Errorf("CreateJobObjectW: %v", err)
	}
	info := jobExtendedLimits{}
	info.Basic.LimitFlags = jobLimitKillOnJobClose
	ok, _, err := procSetInformationJob.Call(handle,
		uintptr(jobInfoExtendedLimitInformation),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info))
	if ok == 0 {
		_, _, _ = procCloseHandleObject.Call(handle)
		return 0, fmt.Errorf("SetInformationJobObject: %v", err)
	}
	return syscall.Handle(handle), nil
}

// treeOwner carries the job between start hooks: created before the process
// exists, assigned and resumed after it starts, closed to end the tree.
type treeOwner struct {
	job syscall.Handle
}

func ownBeforeStart(cmd *exec.Cmd) (*treeOwner, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createSuspendedFlag
	job, err := createKillOnCloseJob()
	if err != nil {
		return nil, err
	}
	return &treeOwner{job: job}, nil
}

func (o *treeOwner) afterStart(cmd *exec.Cmd) error {
	h, _, err := procOpenProcessHandle.Call(processAssignAccess, 0, uintptr(cmd.Process.Pid))
	if h == 0 {
		return fmt.Errorf("OpenProcess on the suspended leader: %v", err)
	}
	defer func() { _, _, _ = procCloseHandleObject.Call(h) }()
	ok, _, err := procAssignProcessToJob.Call(uintptr(o.job), h)
	if ok == 0 {
		return fmt.Errorf("AssignProcessToJobObject: %v", err)
	}
	r, _, err := procNtResumeProcess.Call(h)
	if r != 0 {
		return fmt.Errorf("NtResumeProcess: %v", err)
	}
	return nil
}

func (o *treeOwner) release() error {
	r, _, err := procCloseHandleObject.Call(uintptr(o.job))
	if r == 0 {
		return fmt.Errorf("CloseHandle on the job object: %v", err)
	}
	return nil
}
