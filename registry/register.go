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

// RegisterInstance 封装 Consul 服务注册生命周期。
type RegisterInstance struct {
	// client 是 Consul API 客户端。
	client *api.Client
	// meta 保存服务级元数据。
	meta   *micro.Meta
	// conf 保存注册配置。
	conf   *ServiceConf

	// serviceID 记录当前实例在 Consul 中的唯一 ID。
	serviceID string
}

// NewRegister 创建一个 Consul 注册器实例。
func NewRegister(client *api.Client, meta *micro.Meta, conf *ServiceConf) (*RegisterInstance, error) {
	// 客户端不能为空。
	if client == nil {
		return nil, fmt.Errorf(micro.ErrClientIsNilFormat, "consul")
	}
	// 服务元数据不能为空。
	if meta == nil {
		return nil, micro.ErrServiceMetaIsNil
	}
	// 服务配置不能为空。
	if conf == nil {
		return nil, micro.ErrServiceConfIsNil
	}
	// 补齐配置默认值。
	conf.Bootstrap()

	// 构建注册器对象并返回。
	return &RegisterInstance{
		client: client,
		meta:   meta,
		conf:   conf,
	}, nil
}

// Install 将服务实例注册到 Consul。
func (s *RegisterInstance) Install(service *micro.ServiceNode) error {
	// 服务节点不能为空。
	if service == nil {
		return micro.ErrServiceNodeIsNil
	}

	// 将运行时元数据注入节点对象。
	service.Meta = s.meta
	service.Kernel = s.conf.Kernel
	service.Network = s.conf.Network
	service.Weight = s.conf.Weight
	service.RunDate = time.Now().Format(time.DateTime)

	// 优先使用显式实例 ID，否则基于 appId+时间戳生成。
	instanceID := s.conf.InstanceId
	if instanceID == "" {
		instanceID = fmt.Sprintf("%s-%d", s.meta.AppId, time.Now().UnixNano())
	}
	// 组合 Consul Service ID。
	s.serviceID = fmt.Sprintf("%s-%s", s.meta.AppId, instanceID)

	// 解析注册地址与端口。
	address, port, err := splitAddressPort(service.Network.Internal)
	if err != nil {
		return err
	}

	// 序列化方法集合，存入 Meta。
	methods, err := json.Marshal(service.Methods)
	if err != nil {
		return err
	}

	// 序列化完整节点，便于发现端优先还原。
	nodeRaw, err := json.Marshal(service)
	if err != nil {
		return err
	}

	// 构建 Consul 注册对象。
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
		// 使用 TCP 健康检查；TTL 控制检查间隔和自动摘除窗口。
		Check: &api.AgentServiceCheck{
			TCP:                            net.JoinHostPort(address, strconv.Itoa(port)),
			Interval:                       fmt.Sprintf("%ds", s.conf.TTL),
			Timeout:                        "3s",
			DeregisterCriticalServiceAfter: fmt.Sprintf("%ds", s.conf.TTL*3),
		},
	}

	// 提交注册请求到本地 agent。
	return s.client.Agent().ServiceRegister(registration)
}

// Uninstall 将当前实例从 Consul 注销。
func (s *RegisterInstance) Uninstall() error {
	// 无已注册实例时直接返回。
	if s.serviceID == "" {
		return nil
	}
	// 调用注销接口，并清理本地状态。
	err := s.client.Agent().ServiceDeregister(s.serviceID)
	s.serviceID = ""
	return err
}

// splitAddressPort 解析 host:port 结构字符串。
func splitAddressPort(raw string) (string, int, error) {
	// 先用标准库拆分 host 与 port。
	host, portRaw, err := net.SplitHostPort(raw)
	if err != nil {
		return "", 0, err
	}
	// 再把端口从字符串转换为整数。
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return "", 0, err
	}
	// 返回解析结果。
	return host, port, nil
}
