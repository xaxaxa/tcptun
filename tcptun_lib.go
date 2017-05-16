// tcptun contains the library functions and interfaces used with the tcptun
// tunnelling framework.
package tcptun

import (
	"io"
	"net"
)

var BufSize int = 1024*16

// ReadWrite2Closer is the same as ReadWriteCloser, but with separate
// CloseRead() and CloseWrite() functions
type ReadWrite2Closer interface {
	io.ReadWriteCloser
	CloseRead() error
	CloseWrite() error
}

// TransportClient represent the client side of a transport; a transport
// is any mechanism that allows bi-directional multi-stream communication
// to a server, for example (multiple) tcp connections or an SCTP connection.
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

type MultiplexerClient interface {
	// pre-mux side
	HandleConnection(addr string, stream ReadWrite2Closer) error
	// post-mux side
	SetTransport(TransportClient)
	Begin() error
}

type MultiplexerServer interface {
	// pre-demux side
	Servlet
	// post-demux side
	SetGateway(Gateway)
}

// Gateway represents a gateway to the outside world; it is used on
// the server side of a tunnel to connect to final destinations.
type Gateway interface {
	Dial(addr string) (ReadWrite2Closer, error)
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
	for br < (addrLen + 1) {
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

type NoopGateway struct {
	network string
}

func NewNoopGateway(network string) *NoopGateway {
	return &NoopGateway{network}
}
func (this *NoopGateway) Dial(addr string) (ReadWrite2Closer, error) {
	tmp, err := net.Dial(this.network, addr)
	if err != nil {
		return nil, err
	}
	return tmp.(ReadWrite2Closer), nil
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

func ReadAll(r io.Reader, buf []byte) error {
	br := 0
	for br < len(buf) {
		tmp, err := r.Read(buf[br:])
		if tmp <= 0 || err != nil {
			return err
		}
		br += tmp
	}
	return nil
}

// CombinedPipe combines a read pipe and a write pipe into a ReadWrite2Closer;
// it normally represents one "side" of a bidirectional pipe
type CombinedPipe struct {
	ReadPipe  io.ReadCloser
	WritePipe io.WriteCloser
}

func CombinePipe(reader io.ReadCloser, writer io.WriteCloser) CombinedPipe {
	return CombinedPipe{reader, writer}
}
func BidirPipe() (side1 CombinedPipe, side2 CombinedPipe) {
	pipe1r, pipe1w := io.Pipe()
	pipe2r, pipe2w := io.Pipe()
	side1 = CombinePipe(pipe1r, pipe2w)
	side2 = CombinePipe(pipe2r, pipe1w)
	return side1, side2
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
