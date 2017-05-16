package tcptun

import (
	"encoding/binary"
	"log"
	"sync"
	"io"
	"errors"
)

var TransportBufSize int = 1024 * 32

type SimpleMultiplexer struct {
	connections []*SimpleMultiplexer_conn
	protocolConnection SimpleMultiplexerProtocol
	messageBroadcast     chan SimpleMultiplexer_message
	gateway              Gateway
	freelist             []int
	connectionsMutex     sync.Mutex
	loggingPrefix string
}

type SimpleMultiplexer_conn struct {
	stream     ReadWrite2Closer
	readClosed bool
	ch chan SimpleMultiplexer_message
	readUnblockCh chan int
}
func NewSimpleMultiplexer_conn() *SimpleMultiplexer_conn {
	return &SimpleMultiplexer_conn{nil, false, make(chan SimpleMultiplexer_message, 32), make(chan int)}
}
func (this *SimpleMultiplexer_conn) deinitialize() {
	close(this.ch)
	close(this.readUnblockCh)
}


type SimpleMultiplexer_messageType byte

const (
	M_newConnection SimpleMultiplexer_messageType = iota
	M_closeWrite
	M_close
	M_data
	M_noop
)

type SimpleMultiplexer_message struct {
	Type         SimpleMultiplexer_messageType
	ConnectionID int
	Payload      []byte
}
func (this *SimpleMultiplexer_message) Duplicate() SimpleMultiplexer_message {
	return SimpleMultiplexer_message {this.Type, this.ConnectionID, append([]byte(nil), this.Payload...)}
}

type SimpleMultiplexerProtocol interface {
	ReceiveMessages(cb func(msg SimpleMultiplexer_message)) error
	SendMessage(SimpleMultiplexer_message) error
}

func NewSimpleMultiplexer() *SimpleMultiplexer {
	var tmp SimpleMultiplexer
	tmp.messageBroadcast = make(chan SimpleMultiplexer_message, 128)
	return &tmp
}
func (this *SimpleMultiplexer) AllocateConnection() int {
	this.connectionsMutex.Lock()
	if len(this.freelist) == 0 {
		this.connections = append(this.connections, NewSimpleMultiplexer_conn())
		i := len(this.connections) - 1
		this.connectionsMutex.Unlock()
		return i
	}
	i := this.freelist[len(this.freelist)-1]
	this.freelist = this.freelist[0:len(this.freelist)-1]
	this.connectionsMutex.Unlock()
	this.connections[i] = NewSimpleMultiplexer_conn()
	return i
}
func (this *SimpleMultiplexer) DeallocateConnection(i int) {
	this.connections[i] = nil
	this.connectionsMutex.Lock()
	this.freelist = append(this.freelist, i)
	this.connectionsMutex.Unlock()
}

func (this *SimpleMultiplexer) HandleConnection(connID int, conn *SimpleMultiplexer_conn) {
	stream := conn.stream

	// read side
	go func() {
		for {
			buf := make([]byte, BufSize)
			br, err := stream.Read(buf)
			if conn != this.connections[connID] {
				break
			}
			if err != nil && err != io.EOF {
				shouldClose := false
				this.connectionsMutex.Lock()
				if conn.stream != nil {
					shouldClose = true
					conn.stream = nil
				}
				this.connectionsMutex.Unlock()
				if shouldClose {
					this.messageBroadcast <- SimpleMultiplexer_message{M_close, connID, nil}
				}
				break
			}
			if br <= 0 {
				conn.readClosed = true
				this.messageBroadcast <- SimpleMultiplexer_message{M_closeWrite, connID, nil}
				break
			}
			this.messageBroadcast <- SimpleMultiplexer_message{M_data, connID, buf[0:br]}
			<- conn.readUnblockCh
		}
	} ()
	// write side
	log.Println(this.loggingPrefix, "starting writer", connID)
	for {
		msg, ok := <- conn.ch
		if conn != this.connections[connID] || !ok {
			break
		}
		if conn.stream == nil {
			log.Println(this.loggingPrefix, "stream == nil")
			break
		}
		switch msg.Type {
		case M_data:
			WriteAll(stream, msg.Payload)
		case M_closeWrite:
			stream.CloseWrite()
			shouldClose := false
			this.connectionsMutex.Lock()
			if conn.readClosed && conn.stream != nil {
				shouldClose = true
				conn.stream = nil
			}
			this.connectionsMutex.Unlock()

			if shouldClose {
				stream.Close()
				this.messageBroadcast <- SimpleMultiplexer_message{M_close, connID, nil}
			}
		}
	}
	log.Println(this.loggingPrefix, "exiting writer", connID)
}
func (this *SimpleMultiplexer) HandleTransportConnection() error {
	prot := this.protocolConnection
	go prot.ReceiveMessages(func(msg SimpleMultiplexer_message) {
		if msg.Type == M_newConnection {
			log.Println(this.loggingPrefix, "M_newConnection", msg.ConnectionID)
			this.SetupNewConnection(msg.ConnectionID, string(msg.Payload))
			return
		}
		switch msg.Type {
		case M_data:
			log.Println(this.loggingPrefix, "M_data", msg.ConnectionID,"len:", len(msg.Payload))
		case M_closeWrite:
			log.Println(this.loggingPrefix, "M_closeWrite", msg.ConnectionID)
		case M_close:
			log.Println(this.loggingPrefix, "M_close", msg.ConnectionID)
		}
		connID := msg.ConnectionID
		if connID >= len(this.connections) {
			log.Println(this.loggingPrefix, "connID >= len(this.connections)")
			return
		}
		conn := this.connections[connID]
		stream := conn.stream
		switch msg.Type {
		case M_data:
			conn.ch <- msg
		case M_closeWrite:
			conn.ch <- msg
		case M_close:
			shouldClose := false
			this.connectionsMutex.Lock()
			if conn.stream != nil {
				shouldClose = true
				conn.stream = nil
			}
			this.connectionsMutex.Unlock()
			if shouldClose {
				this.connections[connID] = nil
				stream.CloseRead()
				stream.CloseWrite()
				stream.Close()
				conn.deinitialize()
				this.messageBroadcast <- SimpleMultiplexer_message{M_close, connID, nil}
			}
			this.DeallocateConnection(connID)
			break
		}
	})
	for msg := range this.messageBroadcast {
		prot.SendMessage(msg)
		if msg.Type == M_data {
			this.connections[msg.ConnectionID].readUnblockCh <- 0
		}
	}
	return nil
}
func (this *SimpleMultiplexer) HandleNewConnection(connID int, conn *SimpleMultiplexer_conn, addr string) error {
	if this.gateway == nil {
		return errors.New("HandleNewConnection: gateway is nil!")
	}
	stream, err := this.gateway.Dial(addr)
	if this.connections[connID] != conn {
		if stream != nil {
			stream.Close()
		}
		return nil
	}
	conn.stream = stream
	if err != nil {
		this.messageBroadcast <- SimpleMultiplexer_message{M_close, connID, nil}
	}
	if stream != nil {
		this.HandleConnection(connID, conn)
	}
	return err
}

func (this *SimpleMultiplexer) SetupNewConnection(connID int, addr string) error {
	this.connectionsMutex.Lock()
	for connID >= len(this.connections) {
		this.connections = append(this.connections, nil)
	}
	this.connectionsMutex.Unlock()
	oldConn := this.connections[connID]
	this.connections[connID] = NewSimpleMultiplexer_conn()
	if oldConn != nil {
		if oldConn.stream != nil {
			oldConn.stream.CloseWrite()
			oldConn.stream.CloseRead()
			oldConn.stream.Close()
		}
		oldConn.deinitialize()
	}
	go this.HandleNewConnection(connID, this.connections[connID], addr)
	return nil
}

type SimpleMultiplexerClient struct {
	m *SimpleMultiplexer
	t TransportClient
}

func NewSimpleMultiplexerClient() *SimpleMultiplexerClient {
	return &SimpleMultiplexerClient{nil, nil}
}
func (this *SimpleMultiplexerClient) HandleConnection(addr string, stream ReadWrite2Closer) error {
	id := this.m.AllocateConnection()
	this.m.connections[id].stream = stream
	this.m.messageBroadcast <- SimpleMultiplexer_message{M_newConnection, id, []byte(addr)}
	this.m.HandleConnection(id, this.m.connections[id])
	return nil
}

func (this *SimpleMultiplexerClient) SetTransport(t TransportClient) {
	this.t = t
}

func (this *SimpleMultiplexerClient) Begin() error {
	this.m = NewSimpleMultiplexer()
	this.m.loggingPrefix="[client]"
	stream, err := this.t.Connect()
	if err != nil {
		return err
	}
	prot := &SimpleMultiplexerProtocol_default{}
	this.m.protocolConnection = prot
	prot.Begin(stream)
	go this.m.HandleTransportConnection()
	return nil
}

type SimpleMultiplexerServer struct {
	//mux *SimpleMultiplexer
	gateway Gateway
}

func NewSimpleMultiplexerServer() *SimpleMultiplexerServer {
	return &SimpleMultiplexerServer{}
}
func (this *SimpleMultiplexerServer) SetGateway(g Gateway) {
	this.gateway = g
}
func (this *SimpleMultiplexerServer) HandleConnection(stream ReadWrite2Closer) error {
	mux := NewSimpleMultiplexer()
	mux.loggingPrefix="[server]"
	prot := &SimpleMultiplexerProtocol_default{}
	prot.Begin(stream)
	mux.protocolConnection = prot
	mux.gateway = this.gateway
	return mux.HandleTransportConnection()
}

/*
 protocol packet format:
   bytes	description
   0-1		connection id
   2-3		payload length
   4		command type
   5-		payload
*/

type SimpleMultiplexerProtocol_default struct {
	stream ReadWrite2Closer
}

var SimpleMultiplexerProtocol_default_headerSize int = 5

func (this *SimpleMultiplexerProtocol_default) Begin(stream ReadWrite2Closer) error {
	this.stream = stream
	return nil
}
func (this *SimpleMultiplexerProtocol_default) End() {
	this.stream.Close()
}

func (this *SimpleMultiplexerProtocol_default) ReceiveMessages(cb func(SimpleMultiplexer_message)) error {
	for {
		buf := make([]byte, TransportBufSize)
		br, err := this.stream.Read(buf)
		if br <= 0 || err != nil {
			return err
		}
		this.processPackets(buf[0:br], cb)
	}
}
func (this *SimpleMultiplexerProtocol_default) SendMessage(msg SimpleMultiplexer_message) error {
	// TODO: optimize this
	buf := make([]byte, SimpleMultiplexerProtocol_default_headerSize+len(msg.Payload))
	copy(buf[SimpleMultiplexerProtocol_default_headerSize:], msg.Payload)
	binary.BigEndian.PutUint16(buf[0:2], uint16(msg.ConnectionID))
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(msg.Payload)))
	buf[4] = byte(msg.Type)
	return WriteAll(this.stream, buf)
}

func (this *SimpleMultiplexerProtocol_default) processPackets(buf []byte, cb func(SimpleMultiplexer_message)) error {
	for len(buf) >= SimpleMultiplexerProtocol_default_headerSize {
		var msg SimpleMultiplexer_message
		msg.ConnectionID = int(binary.BigEndian.Uint16(buf[0:2]))
		payloadLen := int(binary.BigEndian.Uint16(buf[2:4]))
		msg.Type = SimpleMultiplexer_messageType(buf[4])
		if SimpleMultiplexerProtocol_default_headerSize+payloadLen <= len(buf) {
			// entire payload is in the buffer
			msg.Payload = buf[5 : payloadLen+5]
			cb(msg)
			buf = buf[payloadLen+5:]
		} else {
			// there is more payload data to be read
			msg.Payload = make([]byte, payloadLen)
			copy(msg.Payload, buf[5:])
			bytesCopied := len(buf) - 5
			err := ReadAll(this.stream, msg.Payload[bytesCopied:])
			if err == nil {
				cb(msg)
			}
			return err
		}
	}
	return nil
}
