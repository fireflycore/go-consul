package registry

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	micro "github.com/fireflycore/go-micro/registry"
	"github.com/hashicorp/consul/api"
)

type RegisterInstance struct {
	client *api.Client
	meta   *micro.Meta
	conf   *micro.ServiceConf

	serviceID string
}

func NewRegister(client *api.Client, meta *micro.Meta, conf *micro.ServiceConf) (*RegisterInstance, error) {
	if client == nil {
		return nil, fmt.Errorf(micro.ErrClientIsNilFormat, "consul")
	}
	if meta == nil {
		return nil, micro.ErrServiceMetaIsNil
	}
	if conf == nil {
		return nil, micro.ErrServiceConfIsNil
	}
	conf.Bootstrap()

	return &RegisterInstance{
		client: client,
		meta:   meta,
		conf:   conf,
	}, nil
}

func (s *RegisterInstance) Install(service *micro.ServiceNode) error {
	if service == nil {
		return micro.ErrServiceNodeIsNil
	}

	service.Meta = s.meta
	service.Kernel = s.conf.Kernel
	service.Network = s.conf.Network
	service.Weight = s.conf.Weight
	service.RunDate = time.Now().Format(time.DateTime)

	instanceID := s.conf.InstanceId
	if instanceID == "" {
		instanceID = fmt.Sprintf("%s-%d", s.meta.AppId, time.Now().UnixNano())
	}
	s.serviceID = fmt.Sprintf("%s-%s", s.meta.AppId, instanceID)

	address, port, err := splitAddressPort(service.Network.Internal)
	if err != nil {
		return err
	}

	methods, err := json.Marshal(service.Methods)
	if err != nil {
		return err
	}

	nodeRaw, err := json.Marshal(service)
	if err != nil {
		return err
	}

	registration := &api.AgentServiceRegistration{
		ID:      s.serviceID,
		Name:    s.meta.AppId,
		Address: address,
		Port:    port,
		Meta: map[string]string{
			"env":              s.meta.Env,
			"app_id":           s.meta.AppId,
			"version":          s.meta.Version,
			"network_sn":       service.Network.SN,
			"network_external": service.Network.External,
			"kernel_language":  service.Kernel.Language,
			"kernel_version":   service.Kernel.Version,
			"weight":           strconv.Itoa(service.Weight),
			"run_date":         service.RunDate,
			"methods":          string(methods),
			"node":             string(nodeRaw),
		},
		Tags: []string{
			s.meta.Env,
			s.meta.Version,
		},
		Check: &api.AgentServiceCheck{
			TCP:                            net.JoinHostPort(address, strconv.Itoa(port)),
			Interval:                       fmt.Sprintf("%ds", s.conf.TTL),
			Timeout:                        "3s",
			DeregisterCriticalServiceAfter: fmt.Sprintf("%ds", s.conf.TTL*3),
		},
	}

	return s.client.Agent().ServiceRegister(registration)
}

func (s *RegisterInstance) Uninstall() error {
	if s.serviceID == "" {
		return nil
	}
	err := s.client.Agent().ServiceDeregister(s.serviceID)
	s.serviceID = ""
	return err
}

func splitAddressPort(raw string) (string, int, error) {
	host, portRaw, err := net.SplitHostPort(raw)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
