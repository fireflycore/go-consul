package agent

import (
	"testing"

	"google.golang.org/grpc"
)

// TestNewServiceRegistrationFromGRPC 验证 gRPC service desc 会被正确映射成服务注册描述。
func TestNewServiceRegistrationFromGRPC(t *testing.T) {
	// 构造一个最小 gRPC service desc，模拟业务服务已注册的方法。
	rawServices := []*grpc.ServiceDesc{
		{
			ServiceName: "acme.auth.v1.AuthService",
			Methods: []grpc.MethodDesc{
				{
					MethodName: "Login",
				},
			},
		},
	}
	// 用最小 service options 构造待测描述。
	descriptor, err := NewServiceRegistrationFromGRPC(GRPCDescriptorOptions{
		AppId:          "10001",
		ServiceName:    "auth",
		Namespace:      "default",
		DNS:            "auth.default.svc.cluster.local",
		Env:            "prod",
		Port:           9090,
		Protocol:       "grpc",
		Version:        "v1.0.0",
		ServiceOptions: &ServiceOptions{},
		RawServices:    rawServices,
	})
	if err != nil {
		t.Fatalf("new descriptor from grpc failed: %v", err)
	}
	// 描述中应保留逻辑服务名与完整 method path。
	if got, want := descriptor.ServiceName, "auth"; got != want {
		t.Fatalf("unexpected service name: got=%s want=%s", got, want)
	}
	if got, want := len(descriptor.Node.Methods), 1; got != want {
		t.Fatalf("unexpected method count: got=%d want=%d", got, want)
	}
	if _, ok := descriptor.Node.Methods["/acme.auth.v1.AuthService/Login"]; !ok {
		t.Fatal("expected full grpc method path to be present")
	}
}

// TestNewServiceLifecycleFromGRPC 验证可以直接从 gRPC 描述组装生命周期桥接对象。
func TestNewServiceLifecycleFromGRPC(t *testing.T) {
	// 使用最小 gRPC 描述直接创建 lifecycle。
	lifecycle, err := NewServiceLifecycleFromGRPC(GRPCDescriptorOptions{
		AppId:       "10002",
		ServiceName: "payment",
		Namespace:   "default",
		DNS:         "payment.default.svc.cluster.local",
		Env:         "prod",
		Port:        8080,
		Protocol:    "grpc",
		Version:     "v2.0.0",
		RawServices: []*grpc.ServiceDesc{
			{
				ServiceName: "acme.payment.v1.PaymentService",
				Methods: []grpc.MethodDesc{
					{
						MethodName: "Pay",
					},
				},
			},
		},
	}, LocalRuntimeOptions{}, LifecycleOptions{
		GracePeriod: "20s",
	})
	if err != nil {
		t.Fatalf("new lifecycle from grpc failed: %v", err)
	}
	// 生命周期对象创建成功后，内部状态对象也应可正常读取。
	if lifecycle == nil {
		t.Fatal("expected non-nil lifecycle")
	}
}
