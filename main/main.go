package main

import (
	"fmt"
	"github.com/matf16/mrpc"
	"log"
	"net"
	"sync"
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
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)

	client, _ := mrpc.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(1 * time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("mrpc req %d", i)
			var reply string
			if err := client.Call("MATF.HELLO", args, &reply); err != nil {
				log.Fatal("call MATF.HELLO error", err)
			}
			log.Println("reply:" + reply)
		}(i)
	}
	wg.Wait()
}
