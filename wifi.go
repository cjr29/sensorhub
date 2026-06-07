package main

import (
	"fmt"
	"os/exec"
)

func getAvailableNetworks() ([]byte, error) {
	// Request NetworkManager to connect to target network
	// nmcli automatically handles turning off the current AP when joining a client network
	cmd := exec.Command("nmcli", "device", "wifi", "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list Wi-Fi networks: %w", err)
	}
	return output, nil
}
