package tcptun

import (
	"math/rand"
	"testing"
	"log"
	"time"
)

func TestSimpleMultiplexerProtocol_default(t *testing.T) {
	TransportBufSize = 8192
	// create a pipe
	side1, side2 := BidirPipe()
	var p1, p2 SimpleMultiplexerProtocol_default
	p1.Begin(side1)
	p2.Begin(side2)

	// generate n random messages with random payloads
	n := 1000
	msgs := make([]SimpleMultiplexer_message, n)
	for i := 0; i < n; i++ {
		msgs[i].Type = SimpleMultiplexer_messageType(rand.Int())
		msgs[i].ConnectionID = rand.Intn(60000)
		msgs[i].Payload = make([]byte, rand.Intn(50000))
		rand.Read(msgs[i].Payload)
	}

	// put messages through
	nReceived := 0
	ch := make(chan int)
	go func() {
		p2.ReceiveMessages(func(msg SimpleMultiplexer_message) {
			i := nReceived
			nReceived += 1
			if i >= n {
				return
			}
			orig := msgs[i]
			if msg.Type != orig.Type || msg.ConnectionID != orig.ConnectionID || len(msg.Payload) != len(orig.Payload) {
				t.Errorf("message %d header corrupted: "+
					"original: Type=%d, ConnectionID=%d, len=%d; "+
					"received: Type=%d, ConnectionID=%d, len=%d",
					i,
					int(orig.Type), orig.ConnectionID, len(orig.Payload),
					int(msg.Type), msg.ConnectionID, len(msg.Payload))
				return
			}
			for j, v := range msg.Payload {
				if v != orig.Payload[j] {
					t.Errorf("message %d payload wrong: byte %d should be %d, is %d",
						i, j, int(orig.Payload[j]), int(v))
					break
				}
			}
		})
		ch <- 0
	}()
	for _, msg := range msgs {
		p1.SendMessage(msg)
	}
	side1.CloseWrite()
	// wait for reader to return
	<-ch

	if nReceived != n {
		t.Errorf("received incorrect number of messages; should be %d, is %d",
			n, nReceived)
	}
}

func TestSimpleMultiplexer(t *testing.T) {
	mux1 := NewSimpleMultiplexer()
	mux2 := NewSimpleMultiplexer()
	side1, side2 := BidirPipe()
	mux1.transportConnections = append(mux1.transportConnections, side1)
	mux2.transportConnections = append(mux2.transportConnections, side2)
	go mux1.HandleTransportConnection(0)
	go mux2.HandleTransportConnection(0)
	
	a1, a2 := BidirPipe()
	b1, b2 := BidirPipe()
	a := mux1.AllocateConnection()
	mux1.connections[a].stream = a2
	b := mux2.AllocateConnection()
	mux2.connections[b].stream = b1
	// connection : a1 <-> b2
	go mux1.HandleConnection(a)
	go mux2.HandleConnection(b)
	
	buf := make([]byte, 1000)
	buf2 := make([]byte, 1000)
	for i:=0; i<100; i++ {
		rand.Read(buf)
		WriteAll(a1, buf)
		err := ReadAll(b2, buf2)
		if err != nil {
			t.Errorf("Read() on client connection returned error: %s", err)
		}
		for j, v := range buf2 {
			if v != buf[j] {
				t.Errorf("message %d payload wrong: byte %d should be %d, is %d",
					i, j, int(buf[j]), int(v))
				break
			}
		}
	}
	log.Println("s")
	ch := make(chan int)
	go func() {
		br, _ := b2.Read(buf2)
		if br != 0 {
			t.Errorf("far side connection did not close when local connection closed; br=%d", br)
		}
		ch <- 0
	} ()
	a1.CloseWrite()
	b2.CloseWrite()
	<- ch
	time.Sleep(100*time.Millisecond)
}
