package xclient

// SelectMode 负载均衡策略
type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
)

type Discovery interface {
	// Refresh 从注册中心更新服务列表
	Refresh() error
	// Update 手动更新服务列表
	Update([]string)
	// Get 根据负载均衡策略，选择一个服务实例
	Get(SelectMode) (string, error)
	// GetAll 返回全部的服务实例
	GetAll() []string
}
