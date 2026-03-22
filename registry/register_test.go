package registry

import "testing"

// TestSplitAddressPort 验证 host:port 解析成功路径。
func TestSplitAddressPort(t *testing.T) {
	// 输入标准地址。
	host, port, err := splitAddressPort("127.0.0.1:8500")
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("unexpected host: %s", host)
	}
	if port != 8500 {
		t.Fatalf("unexpected port: %d", port)
	}
}

// TestSplitAddressPortError 验证非法地址时返回错误。
func TestSplitAddressPortError(t *testing.T) {
	// 缺少端口分隔符时应报错。
	if _, _, err := splitAddressPort("127.0.0.1"); err == nil {
		t.Fatal("expected error for invalid address")
	}
}
