//go:build !windows

package client

import (
	"os"
	"syscall"
)

func setCloexec(w any) {
	if f, ok := w.(*os.File); ok {
		_, _, _ = syscall.RawSyscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_SETFD, syscall.FD_CLOEXEC) //nolint:errcheck // best-effort CLOEXEC
	}
}
