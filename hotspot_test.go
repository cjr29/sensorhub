package main

import (
	"os"
	"strings"
	"testing"
)

func TestSaveSSIDConfigDoesNotStorePassword(t *testing.T) {
	path := t.TempDir() + "/sensorhub.conf"
	if err := saveSSIDConfig(path, "Home WiFi"); err != nil {
		t.Fatalf("saveSSIDConfig() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); got != `{"ssid":"Home WiFi"}` {
		t.Fatalf("config = %s, want SSID-only JSON", got)
	}
}

func TestLoadSSIDConfigRemovesLegacyPassword(t *testing.T) {
	path := t.TempDir() + "/sensorhub.conf"
	legacyConfig := []byte(`{"ssid":"Home WiFi","pwd":"secret-password"}`)

	config, err := loadSSIDConfig(path, legacyConfig)
	if err != nil {
		t.Fatalf("loadSSIDConfig() error = %v", err)
	}
	if config.SSID != "Home WiFi" {
		t.Fatalf("SSID = %q, want %q", config.SSID, "Home WiFi")
	}

	sanitizedConfig, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(sanitizedConfig), "secret-password") ||
		strings.Contains(string(sanitizedConfig), `"pwd"`) {
		t.Fatalf("legacy password remains in config: %s", sanitizedConfig)
	}
}
