package registry

import "testing"

func TestSplitAddressPort(t *testing.T) {
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

func TestSplitAddressPortError(t *testing.T) {
	if _, _, err := splitAddressPort("127.0.0.1"); err == nil {
		t.Fatal("expected error for invalid address")
	}
}
