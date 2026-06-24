package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGatewayManifestNormalizesAndValidates(t *testing.T) {
	manifest := &GatewayManifest{
		Schema:        GatewayManifestSchema,
		DescriptorRef: " https://minio.exmple.com/descriptor/auth/v0.0.1.pb ",
		Services: []GatewayManifestService{
			{
				Name: " acme.auth.v1.AuthService ",
				Methods: []string{
					" /acme.auth.v1.AuthService/Logout ",
					"/acme.auth.v1.AuthService/Login",
					"/acme.auth.v1.AuthService/Login",
				},
			},
		},
		Routes: []HTTPRoute{
			{
				HTTPMethod: "get",
				Path:       " /v1/auth/login ",
				FullMethod: " /acme.auth.v1.AuthService/Login ",
			},
		},
	}
	path := writeTestGatewayManifest(t, manifest)

	loaded, err := LoadGatewayManifest(path)
	if err != nil {
		t.Fatalf("load gateway manifest failed: %v", err)
	}
	if got, want := loaded.DescriptorRef, "https://minio.exmple.com/descriptor/auth/v0.0.1.pb"; got != want {
		t.Fatalf("unexpected descriptor ref: got=%s want=%s", got, want)
	}
	if got, want := len(loaded.MethodPaths()), 2; got != want {
		t.Fatalf("unexpected method count: got=%d want=%d", got, want)
	}
	if got, want := loaded.Routes[0].HTTPMethod, "GET"; got != want {
		t.Fatalf("unexpected http method: got=%s want=%s", got, want)
	}
}

func TestLoadGatewayManifestRejectsMissingAndInvalidFiles(t *testing.T) {
	if _, err := LoadGatewayManifest(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected missing manifest error")
	}

	invalidJSONPath := filepath.Join(t.TempDir(), "gateway.manifest.json")
	if err := os.WriteFile(invalidJSONPath, []byte(`{"schema":`), 0o600); err != nil {
		t.Fatalf("write invalid manifest failed: %v", err)
	}
	if _, err := LoadGatewayManifest(invalidJSONPath); err == nil {
		t.Fatal("expected invalid json error")
	}

	unknownFieldPath := filepath.Join(t.TempDir(), "gateway.manifest.json")
	if err := os.WriteFile(unknownFieldPath, []byte(`{"schema":"firefly.gateway.manifest.v1","services":[],"legacy":true}`), 0o600); err != nil {
		t.Fatalf("write unknown field manifest failed: %v", err)
	}
	if _, err := LoadGatewayManifest(unknownFieldPath); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestGatewayManifestValidationRejectsContractViolations(t *testing.T) {
	tests := []struct {
		name     string
		manifest GatewayManifest
	}{
		{
			name: "wrong_schema",
			manifest: GatewayManifest{
				Schema: "firefly.gateway.manifest.v0",
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
				},
			},
		},
		{
			name: "empty_services",
			manifest: GatewayManifest{
				Schema: GatewayManifestSchema,
			},
		},
		{
			name: "method_without_slash",
			manifest: GatewayManifest{
				Schema: GatewayManifestSchema,
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"acme.auth.v1.AuthService/Login"}},
				},
			},
		},
		{
			name: "method_not_belong_to_service",
			manifest: GatewayManifest{
				Schema: GatewayManifestSchema,
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.user.v1.UserService/Get"}},
				},
			},
		},
		{
			name: "duplicate_service",
			manifest: GatewayManifest{
				Schema: GatewayManifestSchema,
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Logout"}},
				},
			},
		},
		{
			name: "route_without_descriptor_ref",
			manifest: GatewayManifest{
				Schema: GatewayManifestSchema,
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
				},
				Routes: []HTTPRoute{
					{HTTPMethod: "GET", Path: "/v1/auth/login", FullMethod: "/acme.auth.v1.AuthService/Login"},
				},
			},
		},
		{
			name: "unsupported_descriptor_ref_scheme",
			manifest: GatewayManifest{
				Schema:        GatewayManifestSchema,
				DescriptorRef: "oci://artifact-registry/firefly/descriptors/auth:v1.0.0",
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
				},
				Routes: []HTTPRoute{
					{HTTPMethod: "GET", Path: "/v1/auth/login", FullMethod: "/acme.auth.v1.AuthService/Login"},
				},
			},
		},
		{
			name: "s3_descriptor_ref_without_key",
			manifest: GatewayManifest{
				Schema:        GatewayManifestSchema,
				DescriptorRef: "s3://descriptor",
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
				},
				Routes: []HTTPRoute{
					{HTTPMethod: "GET", Path: "/v1/auth/login", FullMethod: "/acme.auth.v1.AuthService/Login"},
				},
			},
		},
		{
			name: "s3_descriptor_ref_without_bucket",
			manifest: GatewayManifest{
				Schema:        GatewayManifestSchema,
				DescriptorRef: "s3:///auth/v0.1.0.pb",
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
				},
				Routes: []HTTPRoute{
					{HTTPMethod: "GET", Path: "/v1/auth/login", FullMethod: "/acme.auth.v1.AuthService/Login"},
				},
			},
		},
		{
			name: "s3_descriptor_ref_with_query",
			manifest: GatewayManifest{
				Schema:        GatewayManifestSchema,
				DescriptorRef: "s3://descriptor/auth/v0.1.0.pb?endpoint=https://minio.example.com",
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
				},
				Routes: []HTTPRoute{
					{HTTPMethod: "GET", Path: "/v1/auth/login", FullMethod: "/acme.auth.v1.AuthService/Login"},
				},
			},
		},
		{
			name: "route_full_method_missing",
			manifest: GatewayManifest{
				Schema:        GatewayManifestSchema,
				DescriptorRef: "https://minio.exmple.com/descriptor/auth/v0.0.1.pb",
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
				},
				Routes: []HTTPRoute{
					{HTTPMethod: "GET", Path: "/v1/auth/logout", FullMethod: "/acme.auth.v1.AuthService/Logout"},
				},
			},
		},
		{
			name: "duplicate_route",
			manifest: GatewayManifest{
				Schema:        GatewayManifestSchema,
				DescriptorRef: "https://minio.exmple.com/descriptor/auth/v0.0.1.pb",
				Services: []GatewayManifestService{
					{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
				},
				Routes: []HTTPRoute{
					{HTTPMethod: "GET", Path: "/v1/auth/login", FullMethod: "/acme.auth.v1.AuthService/Login"},
					{HTTPMethod: "GET", Path: "/v1/auth/login", FullMethod: "/acme.auth.v1.AuthService/Login"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := tc.manifest
			if err := manifest.NormalizeAndValidate(); err == nil {
				t.Fatal("expected manifest validation error")
			}
		})
	}
}

func TestGatewayManifestAllowsGRPCOnlyWithoutDescriptorRef(t *testing.T) {
	manifest := GatewayManifest{
		Schema: GatewayManifestSchema,
		Services: []GatewayManifestService{
			{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
		},
	}
	if err := manifest.NormalizeAndValidate(); err != nil {
		t.Fatalf("expected grpc-only manifest to be valid, got: %v", err)
	}
	if got := manifest.DescriptorRef; got != "" {
		t.Fatalf("unexpected descriptor ref: %s", got)
	}
	if got, want := manifest.MethodPaths()[0], "/acme.auth.v1.AuthService/Login"; got != want {
		t.Fatalf("unexpected method: got=%s want=%s", got, want)
	}
}

func TestGatewayManifestAllowsGRPCOnlyWithDescriptorRef(t *testing.T) {
	manifest := GatewayManifest{
		Schema:        GatewayManifestSchema,
		DescriptorRef: " https://minio.exmple.com/descriptor/auth/v0.0.1.pb ",
		Services: []GatewayManifestService{
			{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
		},
	}
	if err := manifest.NormalizeAndValidate(); err != nil {
		t.Fatalf("expected grpc-only manifest with descriptor_ref to be valid, got: %v", err)
	}
	if got, want := manifest.DescriptorRef, "https://minio.exmple.com/descriptor/auth/v0.0.1.pb"; got != want {
		t.Fatalf("unexpected descriptor ref: got=%s want=%s", got, want)
	}
}

func TestGatewayManifestAllowsS3DescriptorRef(t *testing.T) {
	manifest := GatewayManifest{
		Schema:        GatewayManifestSchema,
		DescriptorRef: " s3://descriptor/auth/v0.1.0.pb ",
		Services: []GatewayManifestService{
			{Name: "acme.auth.v1.AuthService", Methods: []string{"/acme.auth.v1.AuthService/Login"}},
		},
		Routes: []HTTPRoute{
			{HTTPMethod: "post", Path: "/v1/auth/login", FullMethod: "/acme.auth.v1.AuthService/Login"},
		},
	}
	if err := manifest.NormalizeAndValidate(); err != nil {
		t.Fatalf("expected s3 descriptor_ref to be valid, got: %v", err)
	}
	if got, want := manifest.DescriptorRef, "s3://descriptor/auth/v0.1.0.pb"; got != want {
		t.Fatalf("unexpected descriptor ref: got=%s want=%s", got, want)
	}
}

func TestGatewayManifestAllowsHTTPProxyRouteWithoutDescriptorRef(t *testing.T) {
	manifest := GatewayManifest{
		Schema: GatewayManifestSchema,
		Services: []GatewayManifestService{
			{Name: "acme.webhook.v1.WebhookService", Methods: []string{"/acme.webhook.v1.WebhookService/Ping"}},
		},
		Routes: []HTTPRoute{
			{HTTPMethod: "post", Path: " /v1/webhook/callback ", UpstreamPath: " /internal/callback ", StripPrefix: false},
		},
	}
	if err := manifest.NormalizeAndValidate(); err != nil {
		t.Fatalf("expected http proxy manifest to be valid, got: %v", err)
	}
	if got := manifest.DescriptorRef; got != "" {
		t.Fatalf("unexpected descriptor ref: %s", got)
	}
	if got, want := manifest.Routes[0].FullMethod, ""; got != want {
		t.Fatalf("unexpected full method: got=%s want empty", got)
	}
	if got, want := manifest.Routes[0].HTTPMethod, "POST"; got != want {
		t.Fatalf("unexpected http method: got=%s want=%s", got, want)
	}
	if got, want := manifest.Routes[0].UpstreamPath, "/internal/callback"; got != want {
		t.Fatalf("unexpected upstream path: got=%s want=%s", got, want)
	}
}

func TestGatewayManifestRejectsHTTPProxyRouteWithDescriptorRef(t *testing.T) {
	manifest := GatewayManifest{
		Schema:        GatewayManifestSchema,
		DescriptorRef: "https://minio.exmple.com/descriptor/webhook/v0.0.1.pb",
		Services: []GatewayManifestService{
			{Name: "acme.webhook.v1.WebhookService", Methods: []string{"/acme.webhook.v1.WebhookService/Ping"}},
		},
		Routes: []HTTPRoute{
			{HTTPMethod: "POST", Path: "/v1/webhook/callback", UpstreamPath: "/internal/callback"},
		},
	}
	if err := manifest.NormalizeAndValidate(); err == nil {
		t.Fatal("expected http proxy manifest with descriptor_ref to be rejected")
	}
}
