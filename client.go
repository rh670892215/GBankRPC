package GbankRPC

import (
	"GbankRPC/codec"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrShutdown = errors.New("connection is shut down")

type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Err           error
	// 实现异步调用，方法调用完成后存到管道内
	Done chan *Call
}

func (c *Call) done() {
	c.Done <- c
}

// Client 客户端
type Client struct {
	// 消息编解码器
	cc codec.Codec
	// 协议
	opt *Option
	// 每个请求的消息头
	h *codec.Header
	// 发送的请求编号
	seq uint64
	// 保证消息有序发送，控制 pending 的并发
	sending sync.Mutex
	// 控制 client 的并发
	mu sync.Mutex
	// 缓存未处理的请求
	pending map[uint64]*Call
	// 用户主动关闭
	closed bool
	// 有错误等其他原因导致关闭
	shutdown bool
}

// ClientResult 客户端创建结果
type ClientResult struct {
	client *Client
	err    error
}

// 新建 client 方法类型
type newClientFunc func(net.Conn, *Option) (*Client, error)

// XDial 根据协议的不同调用不同的方法连接服务端，并返回客户端实例
// eg.http@10.0.0.1:8888  tcp@10.0.0.1:9999  unix@10.0.0.1:7777
func XDial(rpcAddr string, opt *Option) (*Client, error) {
	parts := strings.Split(rpcAddr, "@")
	protocol, addr := parts[0], parts[1]

	switch protocol {
	case "http":
		return DialHTTP("tcp", addr, opt)
	default:
		return Dial(protocol, addr, opt)
	}
}

// Dial 根据 network address 连接服务端，返回 client
func Dial(network, address string, opt *Option) (*Client, error) {
	return dialTimeout(NewClient, network, address, opt)
}

// DialHTTP 使用 http 协议连接服务端，返回客户端实例
func DialHTTP(network, address string, opt *Option) (*Client, error) {
	return dialTimeout(NewHTTPClient, network, address, opt)
}

// 设置连接超时时间连接服务端
func dialTimeout(f newClientFunc, network, address string, opt *Option) (*Client, error) {
	var err error
	opt = parseOption(opt)
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	// 防止 time.After 先到达，子协程向无缓冲的 clientRes 赋值，产生 goroutine 泄漏
	finish := make(chan struct{})
	defer close(finish)

	clientRes := make(chan *ClientResult)
	go func() {
		client, err := f(conn, opt)
		select {
		case <-finish:
			close(clientRes)
			return
		case clientRes <- &ClientResult{client: client, err: err}:
			return
		}
	}()

	if opt.ConnectTimeout == 0 {
		res := <-clientRes
		return res.client, res.err
	}

	select {
	case <-time.After(opt.ConnectTimeout):
		return nil, fmt.Errorf("dialTimeout rpc client: connect timeout: expect within %s", opt.ConnectTimeout)
	case res := <-clientRes:
		return res.client, res.err
	}
}

// 解析 option 并设置默认值
func parseOption(opt *Option) *Option {
	if opt == nil {
		return DefaultOption
	}

	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt
}

// NewHTTPClient 新建 http 客户端
func NewHTTPClient(conn net.Conn, opt *Option) (*Client, error) {
	// 发送 CONNECT 请求
	io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath))

	// 接收服务端回复，并进行校验
	rsp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && rsp.Status == connected {
		return NewClient(conn, opt)
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + rsp.Status)
	}
	return nil, err
}

// NewClient 新建 rpc 客户端
func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	f, ok := codec.NewCodecFuncTable[opt.CodecType]
	if !ok {
		return nil, fmt.Errorf("NewClient invalid codec type %s", opt.CodecType)
	}

	// 与服务端协商 option 信息
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("NewClient error.err is ", err)
		conn.Close()
		return nil, err
	}

	client := &Client{
		cc:      f(conn),
		seq:     uint64(1),
		pending: make(map[uint64]*Call),
	}

	go client.receive()
	return client, nil
}

func (c *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = c.cc.ReadHeader(&h); err != nil {
			break
		}

		call := c.removeCall(h.Seq)
		switch {
		case call == nil:
			err = c.cc.ReadBody(nil)
		case h.Err != "":
			err = c.cc.ReadBody(nil)
			call.Err = fmt.Errorf(h.Err)
			call.done()
		default:
			if err = c.cc.ReadBody(call.Reply); err != nil {
				call.Err = err
			}
			call.done()
		}
	}
	// 出现错误，终止全部call
	c.terminateCalls(err)
}

// 移除pending中指定 seq 的call，并返回
func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()

	call := c.pending[seq]
	if call == nil {
		return nil
	}
	delete(c.pending, seq)
	return call
}

// 回调 pending 中全部call，并关闭客户端
func (c *Client) terminateCalls(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.shutdown = true
	for _, call := range c.pending {
		call.Err = err
		call.done()
	}
}

// Call 同步调用 serviceMethod 方法
func (c *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := c.Go(serviceMethod, args, reply, make(chan *Call, 1))

	select {
	case <-ctx.Done():
		c.removeCall(call.Seq)
		return errors.New("rpc client:call failed: " + ctx.Err().Error())
	case callRes := <-call.Done:
		return callRes.Err
	}
}

// Go 异步调用 serviceMethod 方法
func (c *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil || cap(done) == 0 {
		done = make(chan *Call, 1)
	}

	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}

	c.send(call)
	return call
}

// 发送方法调用请求
func (c *Client) send(call *Call) {
	c.sending.Lock()
	defer c.sending.Unlock()

	seq, err := c.registerCall(call)
	if err != nil {
		call.Err = err
		call.done()
		return
	}

	// option 信息在初始化 client 时已经交换过，这里只关注实际数据的传输
	c.h = &codec.Header{
		ServiceMethod: call.ServiceMethod,
		Seq:           seq,
	}

	if err := c.cc.Write(c.h, call.Args); err != nil {
		itemCall := c.removeCall(seq)
		if itemCall != nil {
			itemCall.Err = err
			itemCall.done()
		}
	}
}

// 注册call
func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shutdown || c.closed {
		return 0, ErrShutdown
	}

	call.Seq = c.seq
	c.pending[c.seq] = call
	c.seq++
	return call.Seq, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrShutdown
	}
	c.closed = true
	return c.cc.Close()
}

// IsAvailable 判断客户端是否可达
func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && !c.shutdown
}
