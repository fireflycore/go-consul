package registry

import (
	"encoding/json"
	"testing"

	micro "github.com/fireflycore/go-micro/registry"
	"github.com/hashicorp/consul/api"
)

// TestDecodeServiceNodeFromMetaNode 验证 node 元字段反序列化路径。
func TestDecodeServiceNodeFromMetaNode(t *testing.T) {
	// 构造一个完整节点对象用于序列化。
	rawNode := &micro.ServiceNode{
		Methods: map[string]bool{"/user.UserService/Login": true},
		Meta: &micro.Meta{
			Env:        "prod",
			AppId:      "user-service",
			InstanceId: "ins-1",
			Version:    "v1.0.0",
		},
		Network: &micro.Network{
			Internal: "10.0.0.2:8080",
		},
	}
	raw, err := json.Marshal(rawNode)
	if err != nil {
		t.Fatal(err)
	}

	// 构造 ServiceEntry，并把 node 放在 Meta 中。
	node := decodeServiceNode(&api.ServiceEntry{
		Service: &api.AgentService{
			Service: "user-service",
			Meta: map[string]string{
				"node": string(raw),
			},
		},
	})
	if node == nil {
		t.Fatal("expected node")
	}
	if !node.Methods["/user.UserService/Login"] {
		t.Fatal("method parse failed")
	}
}

// TestBuildEvents 验证快照对比会产生新增与删除事件。
func TestBuildEvents(t *testing.T) {
	// before 包含服务 a。
	before := micro.ServiceDiscover{
		"a": []*micro.ServiceNode{
			{
				Meta:    &micro.Meta{AppId: "a", InstanceId: "ins-a"},
				Network: &micro.Network{Internal: "10.0.0.1:8080"},
			},
		},
	}
	// after 包含服务 b。
	after := micro.ServiceDiscover{
		"b": []*micro.ServiceNode{
			{
				Meta:    &micro.Meta{AppId: "b", InstanceId: "ins-b"},
				Network: &micro.Network{Internal: "10.0.0.2:8080"},
			},
		},
	}

	// 预期两条事件：a 删除 + b 新增。
	events := buildEvents(before, after)
	if len(events) != 2 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
}
