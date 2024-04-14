package main

import (
	"context"
	"github.com/matf16/mrpc"
	"log"
	"net"
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
