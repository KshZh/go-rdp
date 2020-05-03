package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/KshZh/go-rdp/rdp"
)

// go run cli.go > x
// diff x srv.go
func main() {
	cli, err := rdp.NewClient("localhost:8899", rdp.NewParams())
	if err != nil {
		log.Fatal("")
	}
	cli.Write([]byte("srv.go"))
	bytes, err := cli.Read()
	if err != nil {
		log.Fatal("")
	}
	n, err := strconv.Atoi(string(bytes))
	if err != nil {
		log.Fatal("")
	}
	for n > 0 {
		bytes, err := cli.Read()
		if err != nil {
			log.Fatal("")
		}
		fmt.Print(string(bytes))
		n -= len(bytes)
	}
	cli.Close()
}
