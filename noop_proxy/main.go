// noop_proxy accepts tcp connections, gets the original dst address using getsockopt(),
// connects to that address, and pipes data between the client and the server.
// use the iptables REDIRECT target to redirect tcp connections to this server.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"github.com/xaxaxa/tcptun/original_dst"
)

//import "./original_dst"

var listenAddress = flag.String("listen", ":9999", "local port to listen on")
var bufSize = flag.Int("bufsize", 8192, "size of userspace buffer when reading/writing to sockets")

func main() {
	flag.Parse()

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
	spliceSockets(sock, outgoingSock)
}

// spliceSockets reads data from a and writes to b, and vice versa
func spliceSockets(a, b *net.TCPConn) {
	var ch chan error
	go func() { ch <- spliceSocket(a, b) }()
	go func() { ch <- spliceSocket(b, a) }()

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

// spliceSocket reads data from src and writes it to dst
func spliceSocket(src, dst *net.TCPConn) error {
	buf := make([]byte, *bufSize)
	for {
		br, err := src.Read(buf)
		if err != nil {
			return nil
		}
		if br <= 0 {
			return dst.CloseWrite()
		}
		dst.Write(buf[:br])
	}
}
