package main

import (
	"GbankRPC"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

func testHTTP() {
	addr := make(chan string)

	// client
	go func() {
		client, err := GbankRPC.DialHTTP("tcp", <-addr, nil)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer client.Close()

		time.Sleep(time.Second)
		wg := sync.WaitGroup{}
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				args := Arg{Num1: 1, Num2: index}
				var reply int
				client.Call(context.Background(), "Foo.Sum", args, &reply)
				log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
			}(i)
		}
		wg.Wait()
	}()

	// server
	ls, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Println("net listen error", err)
		return
	}

	s := GbankRPC.NewServer()
	addr <- ls.Addr().String()
	foo := Foo{}
	s.RegisterService(foo)
	s.HandleHTTP()
	http.Serve(ls, nil)
}
