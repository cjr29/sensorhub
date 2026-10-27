package main

import (
	"os"
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
