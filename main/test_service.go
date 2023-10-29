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

type Foo struct{}

type Arg struct {
	Num1 int
	Num2 int
}

func (f Foo) Sum(arg Arg, reply *int) error {
	*reply = arg.Num1 + arg.Num2
	return nil
}

func testService() {
	addr := make(chan string)
	// 服务端
	go func() {
		foo := Foo{}
		server := GbankRPC.NewServer()
		if err := server.RegisterService(foo); err != nil {
			return
		}
		ls, err := net.Listen("tcp", ":0")
		if err != nil {
			fmt.Println(err)
			return
		}
		log.Println("start rpc server on", ls.Addr())
		addr <- ls.Addr().String()
		server.Accept(ls)
	}()

	time.Sleep(time.Second)
	// 客户端
	client, err := GbankRPC.Dial("tcp", <-addr, nil)
	if err != nil {
		fmt.Println("GbankRPC Dial", err)
		return
	}
	defer client.Close()

	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			var reply int
			args := &Arg{Num1: 1, Num2: index}
			if err := client.Call(context.Background(),"Foo.Sum", args, &reply); err != nil {
				fmt.Println("client call error", err)
				return
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}
