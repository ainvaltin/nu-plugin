//go:build unix

package nu

import "syscall"

func enterForeground(pgid int64) error {
	return syscall.Setpgid(syscall.Getpid(), int(pgid))
}
