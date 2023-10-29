package main

import (
	"GbankRPC"
	"GbankRPC/xclient"
	"context"
	"log"
	"net"
	"sync"
	"time"
)

type FooXClient struct{}

type ArgXClient struct {
	Num1 int
	Num2 int
}

func (f FooXClient) Sum(arg ArgXClient, reply *int) error {
	*reply = arg.Num1 + arg.Num2
	return nil
}
func (f FooXClient) Sleep(arg ArgXClient, reply *int) error {
	time.Sleep(time.Second * time.Duration(arg.Num1))
	*reply = arg.Num1 + arg.Num2
	return nil
}
func testXClient() {
	addrCh1 := make(chan string)
	addrCh2 := make(chan string)

	go startServer(addrCh1)
	go startServer(addrCh2)

	addr1 := <-addrCh1
	addr2 := <-addrCh2

	call(addr1, addr2)
	broadcast(addr1, addr2)
}

func startServer(addrCh chan string) {
	ls, _ := net.Listen("tcp", ":0")

	s := GbankRPC.NewServer()
	var foo FooXClient
	if err := s.RegisterService(foo); err != nil {
		log.Println(err)
	}

	addrCh <- ls.Addr().String()
	s.Accept(ls)
}

func call(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xClient := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer xClient.Close()

	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			rpcCall(context.Background(), xClient, "FooXClient.Sum", "call",
				&ArgXClient{Num1: index, Num2: index * index})
		}(i)
	}
	wg.Wait()
}

func broadcast(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xClient := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer xClient.Close()

	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			rpcCall(context.Background(), xClient, "FooXClient.Sum", "broadcast", &ArgXClient{Num1: index, Num2: index * index})
			// expect 2 - 5 timeout
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			rpcCall(ctx, xClient, "FooXClient.Sleep", "broadcast", &ArgXClient{Num1: index, Num2: index * index})
		}(i)
	}
	wg.Wait()
}

func rpcCall(ctx context.Context, xc *xclient.XClient, serviceMethod, typ string, args *ArgXClient) {
	var reply int
	var err error

	switch typ {
	case "call":
		err = xc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}

	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}
