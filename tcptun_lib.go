// tcptun contains the library functions and interfaces used with the tcptun
// tunnelling framework.
package tcptun

import (
	"io"
	"net"
)

var BufSize int = 8192

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
	// HandleConnection handles one connection from a tcptun client; it blocks until
	// the connection is closed.
	HandleConnection(ReadWrite2Closer) error
}

type SimpleServlet struct {
}

func (this *SimpleServlet) HandleConnection(stream ReadWrite2Closer) error {
	buf := make([]byte, BufSize)
	br, err := stream.Read(buf)
	if br <= 0 || err != nil {
		stream.Close()
		return err
	}
	addrLen := int(buf[0])
	for br < (addrLen+1) {
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
	WriteAll(dstSock.(*net.TCPConn), buf[addrLen+1:br])
	SpliceSockets(stream, dstSock.(*net.TCPConn))
	return nil
}

// SpliceSockets reads data from a and writes to b, and vice versa
func SpliceSockets(a, b ReadWrite2Closer) {
	ch := make(chan error)
	go func() { ch <- SpliceSocket(a, b) }()
	go func() { ch <- SpliceSocket(b, a) }()
	//log.Println("sss")
	// if any of the two splices return an error, close both sockets right away;
	// if any of the two splices finishes, wait for the other splice to finish too
	err := <-ch
	//log.Println("aaaaa")
	if err == nil {
		<-ch
	}
	a.CloseRead()
	a.CloseWrite()
	a.Close()
	b.CloseRead()
	b.CloseWrite()
	b.Close()
	if err != nil {
		<-ch
	}
	close(ch)
	//log.Println("bbbbb")
}

// SpliceSocket reads data from src and writes it to dst
func SpliceSocket(src, dst ReadWrite2Closer) error {
	buf := make([]byte, BufSize)
	for {
		//log.Println("read...")
		br, err := src.Read(buf)
		//log.Println("read returned ", br)
		if err != nil {
			return err
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

// CombinedPipe combines a read pipe and a write pipe into a ReadWrite2Closer
type CombinedPipe struct {
	ReadPipe  io.ReadCloser
	WritePipe io.WriteCloser
}

func CombinePipe(reader io.ReadCloser, writer io.WriteCloser) CombinedPipe {
	return CombinedPipe{reader, writer}
}
func (this CombinedPipe) Read(p []byte) (n int, err error) {
	return this.ReadPipe.Read(p)
}
func (this CombinedPipe) Write(p []byte) (n int, err error) {
	return this.WritePipe.Write(p)
}
func (this CombinedPipe) CloseRead() error {
	return this.ReadPipe.Close()
}
func (this CombinedPipe) CloseWrite() error {
	return this.WritePipe.Close()
}
func (this CombinedPipe) Close() error {
	err1 := this.ReadPipe.Close()
	err2 := this.WritePipe.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
