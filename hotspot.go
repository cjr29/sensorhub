package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	HotspotConnName = "SensorHubHotspot" // The nmcli profile name for your Pi setup AP
	HotspotSSID     = "SensorHub"
	HotspotPWD      = "ConfigureMe123" // Generic password for the hotspot. Helps reduce chane of user error when configuring WiFi.
	WiFiConnName    = "wifi-home"
	// sudo nmcli device wifi hotspot ifname wlan0 con-name "SensorHubHotspot" ssid SensorHub password ConfigureMe123

	listenAddr     = ":8080"
	refreshSeconds = 480 // Gives user 4 minutes to enter SSID and password
	nmcliTimeout   = 5 * time.Second

	pathToHubConfig = "/var/sensorhub/sensorhub.conf"
)

type WiFi struct {
	SSID string `json:"ssid"`
}

type Network struct {
	SSID     string
	SIGNAL   string
	SECURITY string
}

type SSIDPageData struct {
	Refresh  int
	Networks []Network
	Error    string
}

var currentWiFi = WiFi{}

// Checks to see if WiFi has already been configured, and if so, restarts the connection
func wifiConfigured() bool {
	logger.Debug("Entering wifiConfigured.")
	// Does config file exist?
	_, err := currentWiFi.getSSID()
	if err != nil {
		// WiFi not configured
		logger.Debug("Unable to find or read config file. WiFi not configured.")
		wifi = false
		return false
	} else {
		wifi = true
		logger.Debug(fmt.Sprintf("WiFi preconfigured for SSID %s", currentWiFi.SSID))
		return true
	}
}

// Configures and enables the Pi's native hotspot using nmcli
func startHotspot() error {
	logger.Debug("Entered startHotspot")
	// Be sure that the hotspot connection profile exists, create it if it does not exist
	exists := connectionExists(HotspotConnName)
	if exists {
		logger.Debug("SensorHubHotspot not active. Bring it up.")
		return exec.Command("sudo", "nmcli", "connection", "up", HotspotConnName).Run()
	} else {
		logger.Debug("SensorHubHotspot profile does not exist. Create it.")
		// Create the new hotspot network profile and start it
		err := addHotspotConnection(HotspotConnName, HotspotSSID, HotspotPWD)
		if err != nil {
			logger.Error(fmt.Sprintf("Error creating hotspot connection: %s", err))
			return err
		}
		logger.Debug(fmt.Sprintf("Hotspot connection %s for SSID %s is up.", HotspotConnName, HotspotSSID))
	}
	return nil
}

// Shuts down the temporary hotspot network
func stopHotspot() error {
	// logger.Debug("Entered stopHotspot")
	logger.Debug(fmt.Sprintf("Shut down hotspot with connection name %s", HotspotConnName))
	return exec.Command("sudo", "nmcli", "connection", "down", HotspotConnName).Run()
}

func downConnection(name string) error {
	logger.Debug("Entered downConnection")
	cmd := exec.Command("sudo", "nmcli", "connection", "down", HotspotConnName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

func upConnection(name string) error {
	logger.Debug("Entered upConnection")
	cmd := exec.Command("sudo", "nmcli", "connection", "up", HotspotConnName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

func addWifiConnection(name string, ssid string, pwd string) error {
	logger.Debug("Entered addWifiConnection")
	cmd := exec.Command("sudo", "nmcli",
		"con", "add",
		"type", "wifi",
		"con-name", name,
		"ifname", "wlan0",
		"ssid", ssid,
		"wifi-sec.key-mgmt", "wpa-psk",
		"wifi-sec.psk", pwd,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

func addHotspotConnection(name string, ssid string, pwd string) error {
	logger.Debug("Entered addHotspotConnection")
	cmd := exec.Command("sudo", "nmcli",
		"device", "wifi", "hotspot",
		"ifname", "wlan0",
		"con-name", name,
		"ssid", ssid,
		"password", pwd,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	// Configure connection for WPA-PSK security
	cmd = exec.Command("sudo", "nmcli",
		"con", "modify", name,
		"802-11-wireless.mode", "ap",
		"802-11-wireless.band", "bg",
		"ipv4.method", "shared",
		"ipv6.method", "ignore",
		"wifi-sec.key-mgmt", "wpa-psk",
		"wifi-sec.psk", pwd,
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	return nil
}

// uses nmcli to delete specified connection profile
func deleteConnection(name string) error {
	logger.Debug("Entered deleteConnection")
	var cmd *exec.Cmd
	cmd = exec.Command("sudo", "nmcli", "con", "delete", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

func connectToSSID(ssid string, pwd string) error {
	logger.Debug("Entered connectToSSID.")

	// Check if Hotspot is running and stop it if it is
	if connectionExists(HotspotConnName) {
		active, err := connectionActive(context.Background(), HotspotConnName)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed checking hotspot connection. %s", err))
			return (err)
		}
		if active {
			logger.Debug("Hotspot is active. Stopping it before connecting to new SSID.")
			if err := stopHotspot(); err != nil {
				logger.Error(fmt.Sprintf("Failed to stop hotspot: %v", err))
				return err
			}
		}
	}

	// Use credentials to recreate a clean wifi-home connection profile
	// Add the new connection profile before trying to bring it up
	// Delete existing profile if it exisits
	if connectionExists(WiFiConnName) {
		deleteConnection(WiFiConnName)
	} // Add new connection profile
	err := addWifiConnection(WiFiConnName, ssid, pwd)
	if err != nil {
		// Could not connect to WiFi
		return fmt.Errorf("Failed to connect to %s: %v", ssid, err)
	}
	// WiFi Connected
	logger.Info(fmt.Sprintf("Successfully connected to the WiFi network %s", ssid))
	lightCmd <- GREENON

	return nil
}

// Commands NetworkManager to join the network specified by ssid, pwd
func connectToWiFi(connectionName, ssid string, pwd string) error {
	logger.Debug("Entered connectToWiFi")
	logger.Debug(fmt.Sprintf("Deleting previous WiFi connection %s if it exists.", connectionName))
	err := deleteConnection(connectionName)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to remove exiting wifi connection profile. %s", err))
		return err
	}
	logger.Debug(fmt.Sprintf("Bring up WiFi connection %s", connectionName))
	err = addWifiConnection(connectionName, ssid, pwd)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to created new WiFi connection profile. %s", err))
		return err
	}
	// Connected to WiFi, save SSID for future reboots
	logger.Debug("Connected to wifi now. Persisting SSID.")
	if err := (&WiFi{SSID: ssid}).saveSSID(); err == nil {
		logger.Debug(fmt.Sprintf("WiFi SSID saved to %s", pathToHubConfig))
	} else {
		logger.Error(fmt.Sprintf("Unable to save SSID to config file. Error: %s", err))
		return err
	}
	return nil
}

// ProfileExists returns true if nmcli has a connection profile with the given name.
func connectionExists(name string) bool {
	// Use nmcli to list connection names and check for an exact match.
	logger.Debug(fmt.Sprintf("Entering connectionExists to check existence of %s", name))
	cmd := exec.Command("nmcli", "-t", "-f", "name", "con", "show")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// runNmcli runs nmcli command with timeout and returns stdout or an error.
func runNmcli(args ...string) (string, error) {
	cmd := exec.Command("nmcli", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(nmcliTimeout):
		_ = cmd.Process.Kill()
		return "", contextError("nmcli timed out")
	}
}

func contextError(msg string) error {
	return &customError{msg}
}

type customError struct{ s string }

func (e *customError) Error() string { return e.s }

// parseNmcliScan parses nmcli --terse --fields ... output (colon separated)
func parseNmcliScan(raw string) []Network {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	var nets []Network
	for _, l := range lines {
		if l == "" {
			continue
		}
		parts := strings.Split(l, ":")
		// normalize length
		for len(parts) < 6 {
			parts = append(parts, "")
		}
		nets = append(nets, Network{
			SSID:     parts[0],
			SIGNAL:   parts[1],
			SECURITY: parts[2],
		})
	}
	return nets
}

func (w *WiFi) saveSSID() error {
	return saveSSIDConfig(pathToHubConfig, w.SSID)
}

func saveSSIDConfig(path, ssid string) error {
	config := WiFi{SSID: ssid}
	jsonData, err := json.Marshal(config)
	if err != nil {
		logger.Error("Unable to convert SSID to JSON")
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		logger.Error("Unable to open Wi-Fi configuration file")
		return err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		logger.Error("Unable to secure Wi-Fi configuration file permissions")
		return err
	}
	if _, err := f.Write(jsonData); err != nil {
		logger.Error("Unable to write Wi-Fi configuration file")
		return err
	}
	return f.Sync()
}

func (w *WiFi) getSSID() (bool, error) {
	// Read JSON from file
	jsonData, err := os.ReadFile(pathToHubConfig)
	if err != nil {
		logger.Error("Unable to read the configuration file")
		return false, err
	}
	config, err := loadSSIDConfig(pathToHubConfig, jsonData)
	if err != nil {
		logger.Error("Unable to convert JSON to WiFi struct")
		return false, err
	}
	*w = config
	return true, nil
}

func loadSSIDConfig(path string, jsonData []byte) (WiFi, error) {
	var config struct {
		SSID string `json:"ssid"`
	}
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return WiFi{}, err
	}
	wifiConfig := WiFi{SSID: config.SSID}
	return wifiConfig, nil
}

// Check for live connectivity by pinging an public DNS server
func hasInternet() bool {
	// logger.Debug("Entered hasInternet, attempt ping outside local network.")
	cmd := exec.Command("ping", "-c", "1", "-W", "2", "8.8.8.8")
	err := cmd.Run()
	if err != nil {
		internet = false
		return false
	} else {
		internet = true
		return true
	}
}

// ConnectionActive checks whether a given NetworkManager connection exists and is active.
// It returns (true, nil) if the connection is active, otherwise (false, nil).
func connectionActive(ctx context.Context, connName string) (bool, error) {
	if strings.TrimSpace(connName) == "" {
		return false, fmt.Errorf("connName is empty")
	}
	// Check connection exists:
	existsCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "con", "show")
	var existsOut bytes.Buffer
	existsCmd.Stdout = &existsOut
	existsCmd.Stderr = &existsOut
	if err := existsCmd.Run(); err != nil {
		return false, fmt.Errorf("nmcli con show failed: %w (%s)", err, strings.TrimSpace(existsOut.String()))
	}
	found := false
	for _, line := range strings.Split(existsOut.String(), "\n") {
		if strings.TrimSpace(line) == connName {
			found = true
			break
		}
	}
	if !found {
		return false, nil // not existing -> not active
	}
	// Check if that specific connection is active:
	activeCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "con", "show", "--active")
	var activeOut bytes.Buffer
	activeCmd.Stdout = &activeOut
	activeCmd.Stderr = &activeOut
	if err := activeCmd.Run(); err != nil {
		return false, fmt.Errorf("nmcli con show --active failed: %w (%s)", err, strings.TrimSpace(activeOut.String()))
	}
	for _, line := range strings.Split(activeOut.String(), "\n") {
		// -t produces colon-separated fields by default: NAME:TYPE:DEVICE
		// Name is the first field.
		fields := strings.SplitN(line, ":", 2)
		if len(fields) > 0 && fields[0] == connName {
			return true, nil
		}
	}
	return false, nil
}

// Check for existence of configuration file
func isConfigFile() bool {
	_, err := os.Stat(pathToHubConfig)
	if err == nil {
		return true // File exists
	}
	return false // No config file, return false
}

func restartAvahiDaemon() error {
	logger.Debug("Entered restartAvahiDaemon.")
	cmd := exec.Command("sudo", "systemctl", "restart", "avahi-daemon")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}
