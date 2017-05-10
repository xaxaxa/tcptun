package main

import (
	"flag"
	"log"
	"net"

	"github.com/xaxaxa/tcptun"
)

var listenAddress = flag.String("listen", ":9999", "local port to listen on")
var bufSize = flag.Int("bufsize", 8192, "size of userspace buffer when reading/writing to sockets")
var servlet tcptun.SimpleServlet

func main() {
	flag.Parse()
	tcptun.BufSize = *bufSize

	// create listening socket
	serverSock, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatal("could not listen on ", *listenAddress, ": ", err)
	}

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
	servlet.HandleConnection(sock)
}
