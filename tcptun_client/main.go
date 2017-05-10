package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/xaxaxa/tcptun"
	"github.com/xaxaxa/tcptun/original_dst"
)

var listenAddress = flag.String("listen", ":9999", "local port to listen on")
var bufSize = flag.Int("bufsize", 8192, "size of userspace buffer when reading/writing to sockets")
var transportFlag = flag.String("transport", "cmd:ssh localhost ./tcptun_servlet", "transport type:transport params")

var transport tcptun.TransportClient

func main() {
	flag.Parse()
	tcptun.BufSize = *bufSize

	// create transport
	t := *transportFlag
	i := strings.Index(t, ":")
	if i < 0 {
		log.Fatal("syntax error in transport string: must contain a colon")
	}
	transportType := t[:i]
	switch transportType {
	case "cmd":
		transport = tcptun.NewCmdTransport(t[i+1:])
	case "tcp":
		transport = tcptun.NewTCPTransport(t[i+1:])
	default:
		log.Fatal("unknown transport type: ", transportType)
	}

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

	// invoke the transport
	outgoingStream, err := transport.Connect()
	if err != nil {
		log.Println("transport Connect() error: ", err)
		sock.Close()
		return
	}
	log.Println("connected to ", dst)

	// send the dst address
	dstStringBytes := []byte(dst)
	header := []byte{byte(len(dstStringBytes))}
	header = append(header, dstStringBytes...)
	tcptun.WriteAll(outgoingStream, header)

	// splice the two sockets
	tcptun.SpliceSockets(sock, outgoingStream)
}
