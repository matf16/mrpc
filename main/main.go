package main

import (
	"context"
	"github.com/matf16/mrpc"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer(addr chan string) {
	var f Foo

	if err := mrpc.Register(&f); err != nil {
		log.Fatal("register error:", err)
	}
	l, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatal("net error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	mrpc.HandleHTTP()
	addr <- l.Addr().String()
	_ = http.Serve(l, nil)
}

func call(addr chan string) {
	client, err := mrpc.DialHTTP("tcp", <-addr)
	if err != nil {
		log.Fatal("create client error: ", err)
	}
	defer func() { _ = client.Close() }()

	time.Sleep(1 * time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, f := context.WithTimeout(context.Background(), time.Second)
			defer f()
			//
			args := &Args{i, i * i}
			var reply int
			if err := client.Call(ctx, "Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error ", err)
			}
			log.Printf("reply: %d + %d = %d\n", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go call(addr)
	startServer(addr)
}
