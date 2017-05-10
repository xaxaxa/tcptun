package main

import (
	"log"
	"os"

	"github.com/xaxaxa/tcptun"
)

func main() {
	stdStream := tcptun.CombinePipe(os.Stdin, os.Stdout)
	var servlet tcptun.SimpleServlet
	err := servlet.HandleConnection(stdStream)
	if err != nil {
		log.Println(err)
	}
	stdStream.Close()
}
