package GbankRPC

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClient_dialTimeout(t *testing.T) {
	// 标记测试函数可以并行执行
	t.Parallel()
	f := func(conn net.Conn, opt *Option) (*Client, error) {
		defer conn.Close()
		time.Sleep(2 * time.Second)
		return nil, nil
	}

	ls, _ := net.Listen("tcp", ":0")

	t.Run("timeout", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", ls.Addr().String(), &Option{ConnectTimeout: time.Second})
		_assert(err != nil && strings.Contains(err.Error(), "connect timeout"), "expect a timeout error")
	})
	t.Run("success", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", ls.Addr().String(), &Option{ConnectTimeout: 0})
		_assert(err == nil, "0 means no limit")
	})
}

type Bar int

func (b Bar) Timeout(argv int, reply *int) error {
	time.Sleep(time.Second * 2)
	return nil
}

func TestClient_Call(t *testing.T) {
	// server
	addrCh := make(chan string)
	// 服务端
	go func() {
		var bar Bar
		s := NewServer()
		if err := s.RegisterService(bar); err != nil {
			return
		}
		ls, err := net.Listen("tcp", ":0")
		if err != nil {
			fmt.Println(err)
			return
		}
		log.Println("start rpc server on", ls.Addr())
		addrCh <- ls.Addr().String()
		s.Accept(ls)
	}()

	addr := <-addrCh
	time.Sleep(time.Second)
	t.Run("client timeout", func(t *testing.T) {
		client, _ := Dial("tcp", addr, &Option{})
		ctx, _ := context.WithTimeout(context.Background(), time.Second)
		var reply int
		err := client.Call(ctx, "Bar.Timeout", 1, &reply)
		_assert(err != nil && strings.Contains(err.Error(), ctx.Err().Error()), "expect a timeout error")
	})
	t.Run("handle timeout", func(t *testing.T) {
		client, _ := Dial("tcp", addr, &Option{HandleTimeout: time.Second})
		var reply int
		err := client.Call(context.Background(), "Bar.Timeout", 1, &reply)
		_assert(err != nil && strings.Contains(err.Error(), "handle timeout"), "expect a timeout error")
	})
	t.Run("handle success", func(t *testing.T) {
		client, _ := Dial("tcp", addr, &Option{HandleTimeout: time.Second * 10})
		var reply int
		err := client.Call(context.Background(), "Bar.Timeout", 1, &reply)
		_assert(err == nil, "expect success")
	})
}

func TestXDail(t *testing.T) {
	ch := make(chan struct{})
	addr := "/tmp/gbankrpc.sock"
	go func() {
		_ = os.Remove(addr)
		l, err := net.Listen("unix", addr)
		if err != nil {
			t.Fatal("failed to listen unix socket")
		}
		ch <- struct{}{}
		s := NewServer()
		s.Accept(l)
	}()
	<-ch
	_, err := XDial("unix@"+addr, nil)
	_assert(err == nil, "failed to connect unix socket")
}
