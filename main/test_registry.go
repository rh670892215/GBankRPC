package main

import (
	"GbankRPC"
	"GbankRPC/registry"
	"GbankRPC/xclient"
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type FooRegistry struct{}

type ArgFooRegistry struct {
	Num1 int
	Num2 int
}

func (f FooRegistry) Sum(arg ArgFooRegistry, reply *int) error {
	*reply = arg.Num1 + arg.Num2
	return nil
}
func (f FooRegistry) Sleep(arg ArgFooRegistry, reply *int) error {
	time.Sleep(time.Second * time.Duration(arg.Num1))
	*reply = arg.Num1 + arg.Num2
	return nil
}

func testRegistry() {
	log.SetFlags(0)
	registryAddr := "http://localhost:6666/gbankrpc/registry"
	var wg sync.WaitGroup
	wg.Add(1)
	go startRegistry(&wg)
	wg.Wait()

	time.Sleep(time.Second)
	wg.Add(2)
	go startServerTestRegistry(registryAddr, &wg)
	go startServerTestRegistry(registryAddr, &wg)
	wg.Wait()

	time.Sleep(time.Second)
	callTestRegistry(registryAddr)
	broadcastTestRegistry(registryAddr)
}

func startRegistry(wg *sync.WaitGroup) {
	ls, _ := net.Listen("tcp", ":6666")
	r := registry.NewGBankRegistry(0)
	r.HandleHTTP("/gbankrpc/registry")
	wg.Done()
	http.Serve(ls, nil)
}

func startServerTestRegistry(registryAddr string, wg *sync.WaitGroup) {
	var foo FooRegistry
	ls, _ := net.Listen("tcp", ":0")
	s := GbankRPC.NewServer()
	s.RegisterService(foo)
	// 开启服务实例向注册中心定时发送心跳检测协程
	registry.HeartBeat(registryAddr, "tcp@"+ls.Addr().String(), 0)
	wg.Done()
	s.Accept(ls)
}

func callTestRegistry(registry string) {
	d := xclient.NewGBankRPCDiscovery(registry, 0)
	xClient := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer xClient.Close()

	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			rpcCallTestRegistry(context.Background(), xClient, "FooRegistry.Sum", "call",
				&ArgFooRegistry{Num1: index, Num2: index * index})
		}(i)
	}
	wg.Wait()
}

func broadcastTestRegistry(registry string) {
	d := xclient.NewGBankRPCDiscovery(registry, 0)
	xClient := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer xClient.Close()

	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			rpcCallTestRegistry(context.Background(), xClient, "FooRegistry.Sum", "broadcast",
				&ArgFooRegistry{Num1: index, Num2: index * index})
			// expect 2 - 5 timeout
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			rpcCallTestRegistry(ctx, xClient, "FooRegistry.Sleep", "broadcast",
				&ArgFooRegistry{Num1: index, Num2: index * index})
		}(i)
	}
	wg.Wait()
}

func rpcCallTestRegistry(ctx context.Context, xc *xclient.XClient, serviceMethod, typ string, args *ArgFooRegistry) {
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
