package xclient

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type GBankRPCDiscovery struct {
	*MultiServerDiscovery
	// 注册中心地址
	registryAddr string
	// 服务列表上次更新时间
	lastUpdateTime time.Time
	// 服务列表超时时间
	timeout time.Duration
}

var defaultTimeout = time.Second * 10

func NewGBankRPCDiscovery(registryAddr string, timeout time.Duration) *GBankRPCDiscovery {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &GBankRPCDiscovery{
		MultiServerDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registryAddr:         registryAddr,
		timeout:              timeout,
	}
}

// Refresh 从注册中心更新服务列表
func (g *GBankRPCDiscovery) Refresh() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.lastUpdateTime.Add(g.timeout).After(time.Now()) {
		// 无需更新
		return nil
	}

	log.Println("rpc registry: refresh servers from registry", g.registryAddr)

	rsp, err := http.Get(g.registryAddr)
	if err != nil {
		log.Println("rpc registry refresh err:", err)
		return err
	}

	servers := strings.Split(rsp.Header.Get("X-GBankRPC-servers"), ",")
	g.servers = make([]string, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server) != "" {
			g.servers = append(g.servers, server)
		}
	}

	g.lastUpdateTime = time.Now()
	return nil
}

// Update 手动更新服务列表
func (g *GBankRPCDiscovery) Update(servers []string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.servers = servers
	g.lastUpdateTime = time.Now()
}

// Get 根据负载均衡策略，选择一个服务实例
func (g *GBankRPCDiscovery) Get(mode SelectMode) (string, error) {
	if err := g.Refresh(); err != nil {
		return "", err
	}
	return g.MultiServerDiscovery.Get(mode)
}

// GetAll 返回全部的服务实例
func (g *GBankRPCDiscovery) GetAll() []string {
	if err := g.Refresh(); err != nil {
		return nil
	}
	return g.MultiServerDiscovery.GetAll()
}
