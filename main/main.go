package main

import (
	"encoding/json"
	"fmt"
	"github.com/matf16/mrpc"
	"github.com/matf16/mrpc/codec"
	"github.com/matf16/mrpc/option"
	"log"
	"net"
	"time"
)

func startServer(addr chan string) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("net error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	mrpc.Accept(l)
}

func main() {
	addr := make(chan string)
	go startServer(addr)

	conn, _ := net.Dial("tcp", <-addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(1 * time.Second)
	_ = json.NewEncoder(conn).Encode(option.DefaultOption)
	cc := codec.NewGobCodec(conn)
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "MATF.Hello",
			Seq:           uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("mrpc req %d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println(reply)
	}
}
