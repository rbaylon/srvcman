//go:build !windows

package updatesmodel

import (
	"errors"
	"syscall"
)

// pidAlive reports whether a process with this pid exists. EPERM means it does
// but belongs to another user, which is the usual case here: the runner is root's.
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
