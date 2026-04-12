package invocation

import (
	microInvocation "github.com/fireflycore/go-micro/invocation"
	"github.com/hashicorp/consul/api"
)

// NewConnectionManager 创建基于 Consul Locator 的连接管理器。
func NewConnectionManager(client *api.Client, conf *Config, options microInvocation.ConnectionManagerOptions) (*microInvocation.ConnectionManager, error) {
	locator, err := NewLocator(client, conf)
	if err != nil {
		return nil, err
	}
	options.Locator = locator
	return microInvocation.NewConnectionManager(options)
}
