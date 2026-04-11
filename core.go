package consul

import (
	"errors"

	"github.com/hashicorp/consul/api"
)

// New 根据入参创建 Consul API 客户端。
func New(c *Config) (*api.Client, error) {
	// 配置不能为空，否则无法构建客户端。
	if c == nil {
		return nil, errors.New("consul: conf is nil")
	}

	// 从官方默认配置开始，按需覆盖字段。
	cfg := api.DefaultConfig()
	// 用户传入 Address 时覆盖默认地址。
	if c.Address != "" {
		cfg.Address = c.Address
	}
	// 用户传入 Scheme 时覆盖默认协议。
	if c.Scheme != "" {
		cfg.Scheme = c.Scheme
	}
	// 用户传入 Datacenter 时覆盖默认数据中心。
	if c.Datacenter != "" {
		cfg.Datacenter = c.Datacenter
	}
	// 用户传入 Token 时启用 ACL 认证。
	if c.Token != "" {
		cfg.Token = c.Token
	}

	// 当 TLS 显式开启时，注入 TLS 参数。
	if c.TLS != nil && c.TLS.Enable {
		cfg.TLSConfig = api.TLSConfig{
			CAFile:             c.TLS.CaFile,
			CertFile:           c.TLS.CertFile,
			KeyFile:            c.TLS.KeyFile,
			InsecureSkipVerify: c.TLS.InsecureSkipVerify,
		}
	}

	// 最终创建并返回 Consul 客户端。
	return api.NewClient(cfg)
}
