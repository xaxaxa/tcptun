
package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/xaxaxa/tcptun"
	"github.com/xaxaxa/tcptun/original_dst"
)

//import "./original_dst"

var listenAddress = flag.String("listen", ":9999", "local port to listen on")
var bufSize = flag.Int("bufsize", 8192, "size of userspace buffer when reading/writing to sockets")
var muxClient tcptun.MultiplexerClient
func main() {
	flag.Parse()
	tcptun.BufSize = *bufSize

	// create listening socket
	serverSock, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatal("could not listen on ", *listenAddress, ": ", err)
	}
	
	// create multiplexer
	muxServer := tcptun.NewSimpleMultiplexerServer()
	muxServer.SetGateway(tcptun.NewNoopGateway("tcp"))
	transport := LoopbackTransport{muxServer}
	muxClient = tcptun.NewSimpleMultiplexerClient()
	muxClient.SetTransport(&transport)
	muxClient.Begin()

	// main accept loop
	for {
		clientSock, err := serverSock.Accept()
		if err != nil {
			log.Fatal("accept() error: ", err)
		}
		go handleClient(clientSock.(*net.TCPConn))
	}
}

// handleClient is called (in a new goroutine) for every connection accepted
func handleClient(sock *net.TCPConn) {
	// get the original destination of the tcp connection, assuming
	// the connection came from an iptables REDIRECT target
	dst, err := original_dst.GetOriginalDst(sock)
	if err != nil {
		log.Println("GetOriginalDst() error: ", err)
		sock.Close()
		return
	}
	fmt.Println("socket original dst: ", dst)

	err = muxClient.HandleConnection(dst, sock)
	if err != nil {
		log.Println("handleClient() error: ", err)
		sock.Close()
		return
	}
}


type LoopbackTransport struct {
	servlet tcptun.Servlet
}
func (this *LoopbackTransport) Connect() (tcptun.ReadWrite2Closer, error) {
	side1, side2 := tcptun.BidirPipe()
	go this.servlet.HandleConnection(side2)
	return side1, nil
}
