package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigPreservesExplicitZeroMTU(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "client.json")
	content := `{
	  "mode": "client",
	  "remote_addr": "49.232.146.200:9000",
	  "tunnel_addr": "100.0.0.20/24",
	  "mtu": 0,
	  "key": "test-key"
	}`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.MTU != 0 {
		t.Fatalf("expected explicit mtu=0 to be preserved for auto-detection, got %d", cfg.MTU)
	}
}

func TestLoadConfigDefaultsMTUWhenMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "client.json")
	content := `{
	  "mode": "client",
	  "remote_addr": "49.232.146.200:9000",
	  "tunnel_addr": "100.0.0.20/24",
	  "key": "test-key"
	}`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.MTU != 1400 {
		t.Fatalf("expected missing mtu to default to 1400, got %d", cfg.MTU)
	}
}

func TestLoadConfigPreservesReplySourceIP(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "server.json")
	content := `{
	  "mode": "server",
	  "local_addr": "49.232.146.200:9000",
	  "reply_source_ip": "10.2.0.12",
	  "tunnel_addr": "100.0.0.1/24",
	  "key": "test-key"
	}`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.ReplySourceIP != "10.2.0.12" {
		t.Fatalf("expected reply_source_ip to be preserved, got %q", cfg.ReplySourceIP)
	}
}

func TestSaveConfigIncludesReplySourceIPForServer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "server.json")

	cfg := &Config{
		Mode:          "server",
		LocalAddr:     "49.232.146.200:9000",
		ReplySourceIP: "10.2.0.12",
		TunnelAddr:    "100.0.0.1/24",
		Key:           "test-key",
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.ReplySourceIP != "10.2.0.12" {
		t.Fatalf("expected saved reply_source_ip, got %q", loaded.ReplySourceIP)
	}
}
