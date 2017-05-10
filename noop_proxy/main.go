// noop_proxy accepts tcp connections, gets the original dst address using getsockopt(),
// connects to that address, and pipes data between the client and the server.
// use the iptables REDIRECT target to redirect tcp connections to this server.
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
	// get the original destination of the tcp connection, assuming
	// the connection came from an iptables REDIRECT target
	dst, err := original_dst.GetOriginalDst(sock)
	if err != nil {
		log.Println("GetOriginalDst() error: ", err)
		sock.Close()
		return
	}
	fmt.Println("socket original dst: ", dst)

	// connect to the original dst
	outgoingSock1, err := net.Dial("tcp", dst)
	if err != nil {
		log.Println("error connecting to ", dst, ": ", err)
		sock.Close()
		return
	}
	outgoingSock, _ := outgoingSock1.(*net.TCPConn)

	// splice the two sockets
	tcptun.SpliceSockets(sock, outgoingSock)
}
