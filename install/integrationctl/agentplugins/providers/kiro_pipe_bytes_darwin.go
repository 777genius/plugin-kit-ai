//go:build darwin

package providers

import "golang.org/x/sys/unix"

// FIONREAD is defined by Darwin's sys/filio.h but is not exported by x/sys.
const darwinFIONREAD = 0x4004667f

func queuedPipeBytes(fd int) (int, error) {
	return unix.IoctlGetInt(fd, darwinFIONREAD)
}
