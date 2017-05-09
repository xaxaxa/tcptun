package tcptun

import (
	"io"
	"net"
)

var bufSize int = 8192

// ReadWrite2Closer is the same as ReadWriteCloser, but with separate
// CloseRead() and CloseWrite() functions
type ReadWrite2Closer interface {
	io.ReadWriteCloser
	CloseRead() error
	CloseWrite() error
}

// TransportClient represent the client side of a transport; a transport
// is any mechanism that allows bi-directional multi-stream communication
// to a server.
type TransportClient interface {
	// Connect returns a new data stream to the server
	Connect() (ReadWrite2Closer, error)
}

// Servlet handles streams from a tcptun client; a custom transport should
// call NewConnection() on the server side whenever TransportClient::Connect()
// is called on the client side, and connect the two streams by any mechanism;
// Servlet will receive commands from the client and contact tcp servers on behalf
// of the client.
type Servlet interface {
	NewConnection(ReadWrite2Closer) error
}

type SimpleServlet struct {
}

func (this *SimpleServlet) NewConnection(stream ReadWrite2Closer) error {
	buf := make([]byte, bufSize)
	br, err := stream.Read(buf)
	if br <= 0 || err != nil {
		stream.Close()
		return err
	}
	addrLen := int(buf[0])
	for (br - 1) < addrLen {
		tmp, err := stream.Read(buf[br:])
		if tmp <= 0 || err != nil {
			stream.Close()
			return err
		}
		br += tmp
	}
	dst_addr := string(buf[1 : addrLen+1])

	dstSock, err := net.Dial("tcp", dst_addr)
	if err != nil {
		stream.Close()
		return err
	}
	WriteAll(dstSock.(*net.TCPConn), buf[addrLen+1:])
	SpliceSockets(stream, dstSock.(*net.TCPConn))
	return nil
}

// SpliceSockets reads data from a and writes to b, and vice versa
func SpliceSockets(a, b ReadWrite2Closer) {
	var ch chan error
	go func() { ch <- SpliceSocket(a, b) }()
	go func() { ch <- SpliceSocket(b, a) }()

	// if any of the two splices return an error, close both sockets right away;
	// if any of the two splices finishes, wait for the other splice to finish too
	err := <-ch
	if err == nil {
		err = <-ch
	}
	close(ch)
	a.Close()
	b.Close()
}

// SpliceSocket reads data from src and writes it to dst
func SpliceSocket(src, dst ReadWrite2Closer) error {
	buf := make([]byte, bufSize)
	for {
		br, err := src.Read(buf)
		if err != nil {
			return nil
		}
		if br <= 0 {
			return dst.CloseWrite()
		}
		WriteAll(dst, buf[:br])
	}
}

func WriteAll(w io.Writer, buf []byte) error {
	bw := 0
	for bw < len(buf) {
		tmp, err := w.Write(buf[bw:])
		if tmp <= 0 || err != nil {
			return err
		}
		bw += tmp
	}
	return nil
}
