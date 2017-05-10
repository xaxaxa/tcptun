package tcptun

import (
	"log"
	"os"
	"os/exec"
)

// CmdTransport is a TransportClient that executes an arbitrary command
// upon request to connect; it ties stdin/stdout to a pipe and returns
// the pipe as a stream. It can be used with ssh to launch tcptun_servlet
// on a ssh server, which will then handle commands from this client.
type CmdTransport struct {
	Command string
}

func NewCmdTransport(cmd string) CmdTransport {
	return CmdTransport{cmd}
}

func (this CmdTransport) Connect() (ReadWrite2Closer, error) {
	cmd := exec.Command(os.Getenv("SHELL"), "-c", "exec "+this.Command)
	rpipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	wpipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	epipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go func() {
		buf := make([]byte, 8192)
		for {
			br, err := epipe.Read(buf)
			if err != nil || br <= 0 {
				return
			}
			WriteAll(os.Stderr, buf[:br])
		}
	}()

	stream := CmdStream{CombinePipe(rpipe, wpipe), cmd}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return stream, nil
}

type CmdStream struct {
	stream ReadWrite2Closer
	cmd    *exec.Cmd
}

func (this CmdStream) Read(p []byte) (n int, err error) {
	return this.stream.Read(p)
}
func (this CmdStream) Write(p []byte) (n int, err error) {
	return this.stream.Write(p)
}
func (this CmdStream) CloseRead() error {
	return this.stream.CloseRead()
}
func (this CmdStream) CloseWrite() error {
	return this.stream.CloseWrite()
}
func (this CmdStream) Close() error {
	log.Println("CmdStream Close()")
	err1 := this.stream.Close()
	err2 := this.cmd.Wait()
	if err1 != nil {
		return err1
	}
	return err2
}
