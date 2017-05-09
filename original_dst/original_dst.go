package original_dst

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"unsafe"
)

type sockaddr struct {
	family uint16
	data   [14]byte
}

const SO_ORIGINAL_DST = 80

// GetOriginalDst returns an intercepted connection's original destination.
func GetOriginalDst(tcpConn *net.TCPConn) (string, error) {
	file, err := tcpConn.File()
	if err != nil {
		return "", fmt.Errorf("File(): %s", err)
	}
	defer file.Close()

	fd := file.Fd()

	var addr sockaddr
	size := uint32(unsafe.Sizeof(addr))
	err = getsockopt(int(fd), syscall.SOL_IP, SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&addr)), &size)
	if err != nil {
		return "", fmt.Errorf("getsockopt: %s", err)
	}

	var ip net.IP
	switch addr.family {
	case syscall.AF_INET:
		ip = addr.data[2:6]
	default:
		return "", errors.New("unrecognized address family")
	}

	port := int(addr.data[0])<<8 + int(addr.data[1])

	return net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
}

func getsockopt(s int, level int, name int, val uintptr, vallen *uint32) (err error) {
	_, _, e1 := syscall.Syscall6(syscall.SYS_GETSOCKOPT, uintptr(s), uintptr(level), uintptr(name), uintptr(val), uintptr(unsafe.Pointer(vallen)), 0)
	if e1 != 0 {
		err = e1
	}
	return
}
