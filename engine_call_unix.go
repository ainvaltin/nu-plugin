//go:build unix

package nu

import (
	"fmt"
	"syscall"
)

/*
On Unix-like operating systems, if the response is Value pipeline data, it
contains an Int which is the process group ID the plugin must join using
setpgid() in order to be in the foreground.
*/
func enterForeground(v Value) error {
	pgid, ok := v.Value.(int64)
	if !ok {
		return fmt.Errorf("expected pgid to be int, got %T", v.Value)
	}
	return syscall.Setpgid(syscall.Getpid(), int(pgid))
}

/*
If the plugin had been requested to change process groups by the response of
EnterForeground, it should also reset that state by calling setpgid(0), since
plugins are normally in their own process group.
*/
func leaveForeground() error {
	return syscall.Setpgid(syscall.Getpid(), 0)
}
