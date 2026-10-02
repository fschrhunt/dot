//go:build linux

package update

import "syscall"

// isatty reports whether fd is a terminal, through the standard ioctl.
func isatty(fd uintptr) bool {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, 0)
	return errno == 0
}
