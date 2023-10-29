package xclient

import (
	"GbankRPC"
	"context"
	"reflect"
	"sync"
)

type XClient struct {
	discovery Discovery
	mu        sync.Mutex
	mode      SelectMode
	opt       *GbankRPC.Option
	clients   map[string]*GbankRPC.Client
}

func NewXClient(discovery Discovery, mode SelectMode, opt *GbankRPC.Option) *XClient {
	return &XClient{discovery: discovery, mode: mode, opt: opt, clients: make(map[string]*GbankRPC.Client)}
}

func (x *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	// 负载均衡，查找待请求的节点
	addr, err := x.discovery.Get(x.mode)
	if err != nil {
		return err
	}

	return x.call(ctx, addr, serviceMethod, args, reply)
}

func (x *XClient) call(ctx context.Context, addr, serviceMethod string, args, reply interface{}) error {
	client, err := x.dial(addr)
	if err != nil {
		return err
	}

	return client.Call(ctx, serviceMethod, args, reply)
}

// 根据地址匹配可达的客户端
func (x *XClient) dial(addr string) (*GbankRPC.Client, error) {
	x.mu.Lock()
	defer x.mu.Unlock()

	client, ok := x.clients[addr]
	if ok && !client.IsAvailable() {
		client.Close()
		client = nil
		delete(x.clients, addr)
	}

	if client == nil {
		newClient, err := GbankRPC.XDial(addr, x.opt)
		if err != nil {
			return nil, err
		}
		client = newClient
		x.clients[addr] = newClient
	}
	return client, nil
}

// Broadcast 将请求广播到全部实例
func (x *XClient) Broadcast(ctx context.Context, serviceMethod string, arg, reply interface{}) error {
	servers := x.discovery.GetAll()

	wg := sync.WaitGroup{}
	mu := sync.Mutex{}
	var e error
	replyDone := reply == nil

	// 调用时产生错误，终止全部调用
	ctx, cancel := context.WithCancel(ctx)
	for _, serverAddr := range servers {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()

			var cloneReply interface{}
			if reply != nil {
				cloneReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}

			err := x.call(ctx, addr, serviceMethod, arg, cloneReply)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && e == nil {
				e = err
				cancel()
			}

			if e == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(cloneReply).Elem())
				replyDone = true
			}

		}(serverAddr)
	}

	wg.Wait()
	return e
}

func (x *XClient) Close() error {
	x.mu.Lock()
	defer x.mu.Unlock()

	for key, client := range x.clients {
		client.Close()
		delete(x.clients, key)
	}
	return nil
}
