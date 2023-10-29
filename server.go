package GbankRPC

import (
	"GbankRPC/codec"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

const MagicNumber = 0x3bef5c

// Option 配置信息
type Option struct {
	// 标记是一个GbankRPC请求
	MagicNumber int
	// 选取的编解码类型
	CodecType codec.Type
	// 连接超时
	ConnectTimeout time.Duration
	// 处理超时
	HandleTimeout time.Duration
}

// DefaultOption 默认配置
var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobCodecType,
	ConnectTimeout: 10 * time.Second,
}

// Request 请求
type Request struct {
	h           codec.Header
	args, reply reflect.Value
	service     *Service
	method      *MethodType
}

// server 服务实例
type server struct {
	serviceTable sync.Map
}

// 无效请求回复
var invalidResponseBody = struct{}{}

// NewServer 新建server
func NewServer() *server {
	return &server{}
}

// Accept 接收每一个连接请求，并进行处理
func (s *server) Accept(ls net.Listener) {
	for {
		conn, err := ls.Accept()
		if err != nil {
			log.Println("server accept error", err)
			return
		}
		go s.ServeConn(conn)
	}
}

// ServeConn 处理单个连接
func (s *server) ServeConn(conn io.ReadWriteCloser) {
	defer conn.Close()

	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("ServerConn error", err)
		return
	}

	// option 字段校验
	if opt.MagicNumber != MagicNumber {
		log.Println("ServerConn opt.MagicNumber not MagicNumber,is ", opt.MagicNumber)
		return
	}

	f := codec.NewCodecFuncTable[opt.CodecType]
	if f == nil {
		log.Println("ServerConn opt.CodecType not define,is ", opt.CodecType)
		return
	}
	s.ServeCodec(f(conn), opt.HandleTimeout)
}

// ServeCodec 根据编解码类型的不同读取连接数据并进行逻辑处理
func (s *server) ServeCodec(cc codec.Codec, timeout time.Duration) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)

	for {
		// 读取 request
		req, err := s.readRequest(cc)
		if err != nil {
			// 读取 request 过程中出现错误，直接返回
			if req == nil {
				break
			}
			req.h.Err = err.Error()
			s.sendResponse(cc, &req.h, invalidResponseBody, sending)
		}
		wg.Add(1)
		go s.handleRequest(cc, sending, wg, req, timeout)
	}
}

// 读取请求信息
func (s *server) readRequest(cc codec.Codec) (*Request, error) {
	// 读取header
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		return nil, err
	}

	req := &Request{h: h}
	service, method, err := s.findServiceMethod(req.h.ServiceMethod)
	if err != nil {
		return nil, err
	}
	req.service = service
	req.method = method

	// 读取body
	req.args = req.method.NewArg()
	req.reply = req.method.NewReply()

	// make sure req.args is pointer
	argsi := req.args.Interface()
	if req.args.Type().Kind() != reflect.Ptr {
		argsi = req.args.Addr().Interface()
	}
	if err := cc.ReadBody(argsi); err != nil {
		log.Println("readRequest read body error.err = ", err)
	}
	return req, nil
}

// 处理请求
func (s *server) handleRequest(cc codec.Codec, sending *sync.Mutex, wg *sync.WaitGroup, req *Request,
	timeout time.Duration) {
	defer wg.Done()

	called := make(chan struct{})
	sent := make(chan struct{})

	// 防止超时后goroutine泄漏
	finish := make(chan struct{})
	defer close(finish)

	go func() {
		err := req.service.Call(req.method, req.args, req.reply)
		select {
		case <-finish:
			close(called)
			close(sent)
			return
		case called <- struct{}{}:
			if err != nil {
				req.h.Err = err.Error()
				s.sendResponse(cc, &req.h, invalidResponseBody, sending)
				sent <- struct{}{}
				return
			}
			s.sendResponse(cc, &req.h, req.reply.Interface(), sending)
			sent <- struct{}{}
		}
	}()

	if timeout == 0 {
		<-called
		<-sent
		return
	}

	select {
	case <-time.After(timeout):
		req.h.Err = fmt.Sprintf("rpc server: request handle timeout:except %d", timeout)
		s.sendResponse(cc, &req.h, invalidResponseBody, sending)
	case <-called:
		<-sent
	}
}

// 回复处理结果
func (s *server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()

	if err := cc.Write(h, body); err != nil {
		log.Println("sendResponse write err", err)
	}
}

// RegisterService 通过传入的 obj 注册service
func (s *server) RegisterService(obj interface{}) error {
	service := NewService(obj)
	if _, ok := s.serviceTable.LoadOrStore(service.name, service); ok {
		return errors.New("RegisterService rpc: service already defined: " + service.name)
	}

	return nil
}

// 查找对应的service及method exp:service.Method
func (s *server) findServiceMethod(serviceMethod string) (*Service, *MethodType, error) {
	parts := strings.Split(serviceMethod, ".")
	if len(parts) != 2 {
		return nil, nil, errors.New("findServiceMethod rpc server: service/Method request ill-formed: " + serviceMethod)
	}

	serviceName := parts[0]
	methodName := parts[1]

	service, ok := s.serviceTable.Load(serviceName)
	if !ok {
		return nil, nil, errors.New("findServiceMethod rpc server: can't find service " + serviceName)
	}

	svc := service.(*Service)
	method, ok := svc.methods[methodName]
	if !ok {
		return nil, nil, errors.New("findServiceMethod rpc server: can't find Method " + methodName)
	}

	return svc, method, nil
}

const (
	defaultRPCPath      = "/gbankrpc/"
	defaultDebugRPCPath = "/gbankrpc/debug"
	connected           = "200 Connected to Gbank RPC"
)

func (s *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, fmt.Sprintf("405 Method not allowed"))
		return
	}

	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}

	io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
	s.ServeConn(conn)
}

// HandleHTTP 注册支持的 http url
func (s *server) HandleHTTP() {
	http.Handle(defaultRPCPath, s)
	http.Handle(defaultDebugRPCPath, &debugServer{s})
}
