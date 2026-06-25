package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/exec"
	"syscall"
	"time"
)

const (
	HotspotConnName = "SensorHubHotspot" // The nmcli profile name for your Pi setup AP
	HotspotSSID     = "SensorHub"
	HotspotPass     = "ConfigureMe123" // Minimum 8 characters
	WiFiConnName    = "wifi-home"
	// sudo nmcli device wifi hotspot ifname wlan0 con-name "SensorHubHotspot" ssid SensorHub password ConfigureMe123
)

// Configuration
const (
	listenAddr     = ":8080"
	refreshSeconds = 5
	nmcliTimeout   = 5 * time.Second
)

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

// HTML form template for connecting to SSID page
const inputformHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>Sensor Hub Wi-Fi Setup</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: Arial, sans-serif; margin: 40px auto; max-width: 400px; padding: 20px; }
        input[type=text], input[type=password] { width: 100%; padding: 12px; margin: 8px 0; box-sizing: border-box; }
        input[type=submit] { width: 100%; background-color: #4CAF50; color: white; padding: 14px; border: none; cursor: pointer; }
    </style>
</head>
<body>
    <h1>Connect Sensor Hub to WiFi</h1>
    <form action="/connect" method="POST">
        <label>Wi-Fi Name (SSID):</label>
        <input type="text" name="ssid" required>
        <label>Password:</label>
        <input type="password" name="password">
        <input type="submit" value="Connect">
    </form>
</body>
</html>`

func loginWifiViaHotspot() {
	logger.Info("Starting Sensor Hub setup hotspot...")
	if err := startHotspot(); err != nil {
		logger.Error(fmt.Sprintf("Failed to start hotspot: %v", err))
		// Generate signal to stop Sensor Hub
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}

	// Create a channel to catch the WiFi login credentials once submitted
	credChan := make(chan [2]string)
	server := startWebServer(credChan)

	// Wait until the user sends credentials via the captive portal web interface
	creds := <-credChan
	currentWiFi.SSID = creds[0]
	currentWiFi.PWD = creds[1]

	logger.Info(fmt.Sprintf("📥 Received credentials for SSID: %s. Shutting down web server to connect to WiFi.", currentWiFi.SSID))

	// Gracefully shut down server so the HTTP response finishes flushing to the browser
	_ = server.Shutdown(context.Background())

	logger.Info("🛑 Stopping hotspot...")
	_ = stopHotspot()

	logger.Info(fmt.Sprintf("⚡ Connecting to network '%s'...", currentWiFi.SSID))
	if err := connectToWiFi(); err != nil {
		logger.Error(fmt.Sprintf("❌ Failed to connect: %v. Re-triggering hotspot loop...", err))
		// Option: Call main() again or loop to safely recover the hotspot state
		return
	}
	logger.Info(fmt.Sprintf("✅ Successfully connected to the new Wi-Fi network %s", currentWiFi.SSID))
}

// Configures and enables the Pi's native hotspot using nmcli
func startHotspot() error {
	// 1. Stop and Delete previous config instance if leftover
	//_ = exec.Command("nmcli", "connection", "delete", HotspotName).Run()
	err := stopHotspot()
	if err != nil {
		logger.Error(" Unable to stop and delete local hotspot")
		return err
	}
	// 2. Create the new hotspot network profile
	// sudo nmcli device wifi hotspot ifname wlan0 con-name "SensorHubHotspot" ssid SensorHub password ConfigureMe123
	createCmd := exec.Command("sudo", "nmcli", "device", "wifi", "hotspot",
		"ifname", "wlan0",
		"con-name", HotspotConnName,
		"ssid", HotspotSSID,
		"password", HotspotPass,
	)
	if err := createCmd.Run(); err != nil {
		logger.Debug(fmt.Sprintf("Failed to create new hotspot with connection name %s", HotspotConnName))
		return fmt.Errorf("nmcli hotspot creation failed: %w", err)
	}
	return nil
}

// Shuts down and deletes the temporary hotspot network
func stopHotspot() error {
	logger.Debug(fmt.Sprintf("Shut down hotspot with connection name %s", HotspotConnName))
	_ = exec.Command("sudo", "nmcli", "connection", "down", HotspotConnName).Run()
	logger.Debug(fmt.Sprintf("Deleting hotspot with connection name %s", HotspotConnName))
	return exec.Command("sudo", "nmcli", "connection", "delete", HotspotConnName).Run()
}

// Commands NetworkManager to join the user-specified network
func connectToWiFi() error {
	var cmd *exec.Cmd
	// Remove old wifi profile and connection
	logger.Debug(fmt.Sprintf("Shutting down previous WiFi connection %s if it exists.", WiFiConnName))
	cmd = exec.Command("sudo", "nmcli", "connection", "down", WiFiConnName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	logger.Debug(fmt.Sprintf("Deleting previous WiFi connection %s if it exists.", WiFiConnName))
	cmd = exec.Command("sudo", "nmcli", "connection", "delete", WiFiConnName)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	// sudo nmcli device wifi hotspot ifname wlan0 con-name "wifi-home" ssid <SSID> password <password>
	// sudo nmcli connection add type wifi con-name HomeWiFi ifname wlan0 ssid MyNetwork wifi-sec.key-mgmt wpa-psk wifi-sec.psk mysecretpassword
	logger.Debug(fmt.Sprintf("Adding new WiFi connection profile for %s", WiFiConnName))
	cmd = exec.Command("sudo", "nmcli", "connection", "add",
		"type", "wifi",
		"con-name", WiFiConnName,
		"ifname", "wlan0",
		"ssid", currentWiFi.SSID,
		"wifi-sec.key-mgmt", "wpa-psk",
		"wifi-sec.psk", currentWiFi.PWD,
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	logger.Debug(fmt.Sprintf("Bring up WiFi connection %s", WiFiConnName))
	cmd = exec.Command("sudo", "nmcli", "connection", "up", WiFiConnName)
	// cmd = exec.Command("sudo", "nmcli", "device", "wifi", "connect", ssid, "password", password)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	// Connected to WiFi, save SSID for future reboots
	logger.Debug("Connected to wifi now. Persisting SSID and PWD.")
	if err := currentWiFi.saveSSIDPWD(); err == nil {
		logger.Debug(fmt.Sprintf("WiFi SSID and PWD saved to %s", pathToHubConfig))
	} else {
		logger.Error(fmt.Sprintf("Unable to save SSID and password to config file. Error: %s", err))
	}
	return nil
}

// Launches a temporary lightweight Go HTTP web server
func startWebServer(credChan chan [2]string) *http.Server {
	mux := http.NewServeMux()
	server := &http.Server{Addr: ":8080", Handler: mux}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, _ := template.New("form").Parse(inputformHTML)
		_ = tmpl.Execute(w, nil)
	})

	mux.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		ssid := r.FormValue("ssid")
		password := r.FormValue("password")

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<h3>Attempting connection to %s...</h3><p>You can close this window. The hotspot will close shortly.</p>", ssid)

		// Send credentials to the main block channel after a brief pause
		go func() {
			time.Sleep(2 * time.Second)
			credChan <- [2]string{ssid, password}
		}()
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	return server
}

// Check for live connectivity by pinging an public DNS server
func hasInternet() bool {
	// cmd := exec.Command("ping", "-c", "1", "8.8.8.8")
	cmd := exec.Command("ping", "-c", "1", "-W", "2", "8.8.8.8")
	return cmd.Run() == nil
}

// func isConnected() bool {
// 	// Quick check to see if the device can route packets to the internet
// 	cmd := exec.Command("ping", "-c", "1", "-W", "2", "8.8.8.8")
// 	err := cmd.Run()
// 	return err == nil
// }
