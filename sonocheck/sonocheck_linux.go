// +build linux

package sonocheck

import (
	"os"
	"syscall"
)

// from /usr/include/asm-generic/socket.h on Linux so it can cross-compile
// should be syscall.SO_NO_CHECK
const so_no_check = 11

func SetNoCheck(fd *os.File) error {
	return syscall.SetsockoptInt(int(fd.Fd()), syscall.SOL_SOCKET, so_no_check, 1)
}
