// +build !linux

package sonocheck

import (
	"errors"
	"os"
)

var err = errors.New("SO_NO_CHECK only supported on Linux")

func SetNoCheck(fd *os.File) error {
	return err
}
