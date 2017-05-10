package tcptun

import (
	"log"
	"net"
)

type TCPTransport struct {
	serverAddress string
}

func NewTCPTransport(addr string) TCPTransport {
	return TCPTransport{addr}
}

func (this TCPTransport) Connect() (ReadWrite2Closer, error) {
	outgoingSock1, err := net.Dial("tcp", this.serverAddress)
	if err != nil {
		log.Println("error connecting to ", this.serverAddress, ": ", err)
		return nil, err
	}
	return outgoingSock1.(*net.TCPConn), nil
}
