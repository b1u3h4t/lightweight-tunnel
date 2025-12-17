package config

import (
	"os"
	"testing"
)

func TestLoadConfigMultiClientDefault(t *testing.T) {
	// Create a temporary config file without multi_client field
	tmpFile, err := os.CreateTemp("", "test_config_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a server config without multi_client field
	configData := `{
  "mode": "server",
  "local_addr": "0.0.0.0:9000",
  "tunnel_addr": "10.0.0.1/24"
}`
	if _, err := tmpFile.WriteString(configData); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Load the config
	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify multi_client defaults to true for server mode
	if !cfg.MultiClient {
		t.Errorf("Expected MultiClient to default to true, got false")
	}

	// Verify MaxClients defaults to 100
	if cfg.MaxClients != 100 {
		t.Errorf("Expected MaxClients to be 100, got %d", cfg.MaxClients)
	}
}

func TestLoadConfigMultiClientExplicitFalse(t *testing.T) {
	// Create a temporary config file with explicit multi_client = false
	tmpFile, err := os.CreateTemp("", "test_config_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a server config with explicit multi_client = false
	configData := `{
  "mode": "server",
  "local_addr": "0.0.0.0:9000",
  "tunnel_addr": "10.0.0.1/24",
  "multi_client": false
}`
	if _, err := tmpFile.WriteString(configData); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Load the config
	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify multi_client respects explicit false value
	if cfg.MultiClient {
		t.Errorf("Expected MultiClient to be false when explicitly set, got true")
	}
}

func TestLoadConfigMultiClientExplicitTrue(t *testing.T) {
	// Create a temporary config file with explicit multi_client = true
	tmpFile, err := os.CreateTemp("", "test_config_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a server config with explicit multi_client = true
	configData := `{
  "mode": "server",
  "local_addr": "0.0.0.0:9000",
  "tunnel_addr": "10.0.0.1/24",
  "multi_client": true
}`
	if _, err := tmpFile.WriteString(configData); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Load the config
	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify multi_client is true
	if !cfg.MultiClient {
		t.Errorf("Expected MultiClient to be true, got false")
	}
}

func TestLoadConfigClientModeNoDefault(t *testing.T) {
	// Create a temporary config file for client mode without multi_client
	tmpFile, err := os.CreateTemp("", "test_config_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a client config without multi_client field
	configData := `{
  "mode": "client",
  "local_addr": "0.0.0.0:9000",
  "remote_addr": "192.168.1.1:9000",
  "tunnel_addr": "10.0.0.2/24"
}`
	if _, err := tmpFile.WriteString(configData); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Load the config
	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify multi_client remains false for client mode (not set to true)
	// Client mode doesn't use multi_client, so it should remain false
	if cfg.MultiClient {
		t.Errorf("Expected MultiClient to remain false for client mode, got true")
	}
}
