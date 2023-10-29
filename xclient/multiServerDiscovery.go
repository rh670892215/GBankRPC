package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type MultiServerDiscovery struct {
	// 记录轮询算法轮询到的位置
	index int
	mu    sync.RWMutex
	// 全部服务实例
	servers []string
	// 随机数种子
	r *rand.Rand
}

func NewMultiServerDiscovery(servers []string) *MultiServerDiscovery {
	res := &MultiServerDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	res.index = res.r.Intn(math.MaxInt32 - 1)
	return res
}

func (m *MultiServerDiscovery) Refresh() error {
	return nil
}

func (m *MultiServerDiscovery) Update(servers []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.servers = servers
}

func (m *MultiServerDiscovery) Get(model SelectMode) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := len(m.servers)
	if n == 0 {
		return "", errors.New("servers is empty")
	}

	switch model {
	case RandomSelect:
		return m.servers[m.r.Intn(n)], nil
	case RoundRobinSelect:
		s := m.servers[m.index%n] // servers could be updated, so mode n to ensure safety
		m.index = (m.index + 1) % n
		return s, nil
	default:
		return "", errors.New("undefined model")
	}
}

func (m *MultiServerDiscovery) GetAll() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]string, len(m.servers), len(m.servers))
	copy(res, m.servers)
	return res
}
