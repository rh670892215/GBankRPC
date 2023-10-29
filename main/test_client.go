package main

import (
	"GbankRPC"
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

func testClient() {
	log.SetFlags(0)
	addr := make(chan string)
	// server
	go func() {
		ls, err := net.Listen("tcp", ":0")
		if err != nil {
			log.Println("net listen error", err)
			return
		}
		log.Println("start rpc server on", ls.Addr())
		s := GbankRPC.NewServer()
		addr <- ls.Addr().String()
		s.Accept(ls)
	}()

	client, err := GbankRPC.Dial("tcp", <-addr, nil)
	defer client.Close()
	if err != nil {
		log.Println(err)
		return
	}

	time.Sleep(time.Second)
	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			var reply string
			args := fmt.Sprintf("gbankrpc req %d", index)
			if err := client.Call(context.Background(), "Foo.Sum", args, &reply); err != nil {
				log.Println(err)
			}
			log.Println(reply)
		}(i)
	}
	wg.Wait()
}
