package sonocheck

import (
	"errors"
	"fmt"
	"net"
	"syscall"
)

func SetNoCheckConn(conn net.Conn) error {
	udpconn, ok := conn.(*net.UDPConn)
	if !ok {
		return errors.New("conn is not net.UDPConn")
	}
	// TOTALLY NON-OBVIOUS: This makes udpconn "non blocking": meaning timeouts don't work
	// fix through a disgusting hack: call SetNonblock on the file descriptor
	fd, err := udpconn.File()
	defer fd.Close() // docs state: "caller's responsibility to close f when finished"
	if err != nil {
		return err
	}
	err = SetNoCheck(fd)
	err2 := syscall.SetNonblock(int(fd.Fd()), true)
	if err != nil {
		return err
	}
	return err2
}

func LogSetNoCheck(conn net.Conn) {
	err := SetNoCheckConn(conn)
	var successMessage string
	if err == nil {
		successMessage = "SUCCESS!"
	} else {
		successMessage = "failed (expected): " + err.Error()
	}
	fmt.Printf("disabling UDP checksums with setsockopt(SO_NO_CHECK): %s\n", successMessage)
}
