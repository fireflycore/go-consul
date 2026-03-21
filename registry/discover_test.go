package registry

import (
	"encoding/json"
	"testing"

	micro "github.com/fireflycore/go-micro/registry"
	"github.com/hashicorp/consul/api"
)

func TestDecodeServiceNodeFromMetaNode(t *testing.T) {
	rawNode := &micro.ServiceNode{
		Methods: map[string]bool{"/user.UserService/Login": true},
		Meta: &micro.Meta{
			Env:     "prod",
			AppId:   "user-service",
			Version: "v1.0.0",
		},
		Network: &micro.Network{
			Internal: "10.0.0.2:8080",
		},
	}
	raw, err := json.Marshal(rawNode)
	if err != nil {
		t.Fatal(err)
	}

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

func TestBuildEvents(t *testing.T) {
	before := micro.ServiceDiscover{
		"a": []*micro.ServiceNode{
			{
				Meta:    &micro.Meta{AppId: "a"},
				Network: &micro.Network{Internal: "10.0.0.1:8080"},
			},
		},
	}
	after := micro.ServiceDiscover{
		"b": []*micro.ServiceNode{
			{
				Meta:    &micro.Meta{AppId: "b"},
				Network: &micro.Network{Internal: "10.0.0.2:8080"},
			},
		},
	}

	events := buildEvents(before, after)
	if len(events) != 2 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
}
