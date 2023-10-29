package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var DefaultTimeOut = 5 * time.Minute

// GBankRegistry 注册中心
type GBankRegistry struct {
	timeout time.Duration
	mu      sync.Mutex
	servers map[string]*ServerItem
}

// ServerItem 服务实例
type ServerItem struct {
	Addr  string
	start time.Time
}

// NewGBankRegistry 新建注册中心，传入设定的超时时间
func NewGBankRegistry(timeout time.Duration) *GBankRegistry {
	if timeout == 0 {
		timeout = DefaultTimeOut
	}

	return &GBankRegistry{
		timeout: timeout,
		servers: make(map[string]*ServerItem),
	}
}

// HandleHTTP 注册http路径
func (g *GBankRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, g)
	log.Println("GBankRegistry registry url :", registryPath)
}

func (g *GBankRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		w.Header().Set("X-GBankRPC-servers", strings.Join(g.getAliveServers(), ","))
	case "POST":
		serverAddr := req.Header.Get("X-GBankRPC-server")
		if serverAddr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		g.putServer(serverAddr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// 注册服务实例
func (g *GBankRegistry) putServer(addr string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	s := g.servers[addr]
	if s == nil {
		g.servers[addr] = &ServerItem{Addr: addr, start: time.Now()}
	} else {
		s.start = time.Now()
	}
}

// 获取全部活跃的服务实例
func (g *GBankRegistry) getAliveServers() []string {
	g.mu.Lock()
	defer g.mu.Unlock()

	var res []string
	for addr, s := range g.servers {
		if g.timeout == 0 || s.start.Add(g.timeout).After(time.Now()) {
			res = append(res, addr)
		} else {
			delete(g.servers, addr)
		}
	}
	sort.Strings(res)
	return res
}

// HeartBeat 定时心跳注册
func HeartBeat(registry, serverAddr string, duration time.Duration) {
	if duration == 0 {
		duration = DefaultTimeOut - time.Duration(1)*time.Minute
	}
	err := sendHeartBeat(registry, serverAddr)

	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C
			err = sendHeartBeat(registry, serverAddr)
		}
		log.Println("HeartBeat error: ", err)
	}()
}

// 发送心跳请求
func sendHeartBeat(registry, serverAddr string) error {
	log.Println(serverAddr, "send heart beat to registry", registry)
	client := &http.Client{}
	req, err := http.NewRequest("POST", registry, nil)
	if err != nil {
		log.Println("rpc server: POST heart beat err:", err)
	}
	req.Header.Set("X-GBankRPC-server", serverAddr)
	if _, err := client.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}
