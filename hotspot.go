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
	HotspotName        = "Hotspot" // The nmcli profile name for your Pi setup AP
	HotspotSSID        = "SensorHub"
	HotspotPass        = "ConfigureMe123" // Minimum 8 characters
	WiFiConnectionName = "wifi-home"
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

// HTML form template for the user configuration page
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

func setupViaHotspot() {
	logger.Info("Starting Sensor Hub setup hotspot...")
	if err := startHotspot(); err != nil {
		logger.Error(fmt.Sprintf("Failed to start hotspot: %v", err))
		// Generate signal to stop Sensor Hub
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}

	// Create a channel to catch the credentials once submitted
	credChan := make(chan [2]string)
	server := startWebServer(credChan)

	// Wait until the user sends credentials via the captive portal web interface
	creds := <-credChan
	targetSSID, targetPass := creds[0], creds[1]

	logger.Info(fmt.Sprintf("📥 Received credentials for SSID: %s. Shutting down web server to connect to WiFi.", targetSSID))

	// Gracefully shut down server so the HTTP response finishes flushing to the browser
	_ = server.Shutdown(context.Background())

	logger.Info("🛑 Stopping hotspot...")
	_ = stopHotspot()

	logger.Info(fmt.Sprintf("⚡ Connecting to network '%s'...", targetSSID))
	if err := connectToWiFi(targetSSID, targetPass); err != nil {
		logger.Error(fmt.Sprintf("❌ Failed to connect: %v. Re-triggering hotspot loop...", err))
		// Option: Call main() again or loop to safely recover the hotspot state
		return
	}

	// Write out the SSID to the persistent config file

	logger.Info(fmt.Sprintf("✅ Successfully connected to the new Wi-Fi network %s", targetSSID))
}

// Check for live connectivity by pinging an public DNS server
// func hasInternet() bool {
// 	cmd := exec.Command("ping", "-c", "1", "8.8.8.8")
// 	return cmd.Run() == nil
// }

// Configures and enables the Pi's native hotspot using nmcli
func startHotspot() error {
	// 1. Delete previous config instance if leftover
	_ = exec.Command("nmcli", "connection", "delete", HotspotName).Run()

	// 2. Create the hotspot network profile
	createCmd := exec.Command("nmcli", "device", "wifi", "hotspot",
		"ssid", HotspotSSID,
		"password", HotspotPass,
		"ifname", "wlan0",
		"con-name", HotspotName,
	)
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("nmcli hotspot creation failed: %w", err)
	}
	return nil
}

// Shuts down and deletes the temporary hotspot network
func stopHotspot() error {
	_ = exec.Command("nmcli", "connection", "down", HotspotName).Run()
	return exec.Command("nmcli", "connection", "delete", HotspotName).Run()
}

// Commands NetworkManager to join the user-specified network
func connectToWiFi(ssid, password string) error {
	var cmd *exec.Cmd
	// Remove old wifi connection
	cmd = exec.Command("sudo", "nmcli", "connection", "delete", "wifi-home")
	logger.Info(fmt.Sprintf("Attempting to connect to SSID: %s", ssid))
	cmd = exec.Command("sudo", "nmcli", "device", "wifi", "connect", ssid, "password", password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	// Connected to WiFi, save SSID for future reboots
	saveSSID(ssid, pathToHubConfig)
	return nil
}

// Launches a temporary lightweight Go HTTP web server
func startWebServer(credChan chan [2]string) *http.Server {
	mux := http.NewServeMux()
	server := &http.Server{Addr: ":80", Handler: mux}

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

// func getAvailableNetworks() ([]byte, error) {
// 	// Request NetworkManager to connect to target network
// 	// nmcli automatically handles turning off the current AP when joining a client network
// 	cmd := exec.Command("nmcli", "device", "wifi", "list")
// 	output, err := cmd.CombinedOutput()
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to list Wi-Fi networks: %w", err)
// 	}
// 	return output, nil
// }
