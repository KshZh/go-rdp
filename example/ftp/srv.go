package main

import (
	"io/ioutil"
	"log"
	"strconv"

	"github.com/KshZh/go-rdp/rdp"
)

func main() {
	srv, err := rdp.NewServer(8899, rdp.NewParams())
	if err != nil {
		log.Fatal("")
	}
	for {
		connID, bytes, err := srv.Read()
		if err != nil {
			continue
		}
		// 获取请求的文件名。
		filename := string(bytes)
		log.Println(filename)
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			continue
		}
		// 先写入文件大小。
		srv.Write(connID, []byte(strconv.Itoa(len(data))))
		i := 0
		for ; i+64 <= len(data); i += 64 {
			srv.Write(connID, data[i:i+64])
		}
		srv.Write(connID, data[i:])
		srv.CloseConn(connID)
	}
}
