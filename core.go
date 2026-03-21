package consul

import (
	"errors"

	"github.com/hashicorp/consul/api"
)

func New(c *Conf) (*api.Client, error) {
	if c == nil {
		return nil, errors.New("consul: conf is nil")
	}

	cfg := api.DefaultConfig()
	if c.Address != "" {
		cfg.Address = c.Address
	}
	if c.Scheme != "" {
		cfg.Scheme = c.Scheme
	}
	if c.Datacenter != "" {
		cfg.Datacenter = c.Datacenter
	}
	if c.Token != "" {
		cfg.Token = c.Token
	}

	if c.TLS != nil && c.TLS.Enable {
		cfg.TLSConfig = api.TLSConfig{
			CAFile:             c.TLS.CaFile,
			CertFile:           c.TLS.CertFile,
			KeyFile:            c.TLS.KeyFile,
			InsecureSkipVerify: c.TLS.InsecureSkipVerify,
		}
	}

	return api.NewClient(cfg)
}
