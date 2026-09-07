package config

import (
	"os"
	"testing"
)

func TestEncodeAuthProxy(t *testing.T) {
	got, err := EncodeAuthProxy(`{"username":"user1","password":"pass1"}`)
	if err != nil {
		t.Fatalf("EncodeAuthProxy() error = %v", err)
	}
	if got != "dXNlcjE6cGFzczE=" {
		t.Fatalf("encoded value = %q, want dXNlcjE6cGFzczE=", got)
	}
}

func TestEncodeAuthProxyRejectsInvalidValues(t *testing.T) {
	cases := []string{"", "{}", `{"username":"","password":"pass1"}`, `{"username":"user1","password":""}`, `{"username":"user1"}`, `{"password":"pass1"}`, `{"username":"user1","password":"pass1","extra":true}`, `{"username":"user1","password":"pass1"} {}`}
	for _, raw := range cases {
		if got, err := EncodeAuthProxy(raw); err == nil || got != "" {
			t.Errorf("EncodeAuthProxy(%q) = %q, %v; want error and empty value", raw, got, err)
		}
	}
}

func TestLoadRequiresAuthProxyOnlyInUse(t *testing.T) {
	t.Setenv("MODE", "use")
	t.Setenv("CF_USE_ACCOUNTS", `[{"name":"a","api_token":"t","account_id":"i"}]`)
	t.Setenv("AUTH_PROXY", "")
	if _, err := Load(); err == nil {
		t.Fatal("use mode should require AUTH_PROXY")
	}

	t.Setenv("MODE", "create")
	_ = os.Unsetenv("AUTH_PROXY")
	if _, err := Load(); err != nil {
		t.Fatalf("create mode should not require AUTH_PROXY: %v", err)
	}
}

func TestLoadUseProducesEncodedValueWithoutRetainingJSON(t *testing.T) {
	t.Setenv("MODE", "use")
	t.Setenv("AUTH_PROXY", `{"username":"user1","password":"pass1"}`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AuthProxyBasic != "dXNlcjE6cGFzczE=" {
		t.Fatalf("AuthProxyBasic = %q, want exact Base64", cfg.AuthProxyBasic)
	}
}
