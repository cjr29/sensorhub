package main

import (
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

const (
	Logging  = true            // Always on for logging. Set level with LogLevel property
	LogLevel = slog.LevelDebug // Change to Error for production
)

const (
	hotspotProfile  = "HubHotspot" // The nmcli profile name for your Pi setup AP
	serverPort      = ":80"
	pathToHubConfig = "/var/sensorhub/sensorhub.config"
)

var logAll *slog.Logger

var tmpl = template.Must(template.New("").ParseGlob("templates/*.html"))

func main() {

	logAll = ConfigureLogging(LogLevel, Logging)
	logAll.Info("Starting Sensor Hub...")

	// Check if sensor hub is already configured to connect to a Wi-Fi network. If not, start the hotspot and web server for setup.
	if configured() {
		logAll.Info("Already configured to connect to Wi-Fi. Now test connection.")
		if isConnected() {
			logAll.Info("Already connected to Wi-Fi. No setup needed.")
		} else {
			logAll.Info("Configured Wi-Fi network not currently reachable. Starting setup hotspot and server...")
			configureViaHotspot()
		}

		logAll.Info("No existing connection detected. Starting setup hotspot and server...")
	} else {
		logAll.Info("WiFi has not been configured. Starting hotspot configuration.")
		configureViaHotspot()
	}
}

// func main() {
// 	logAll = ConfigureLogging(LogLevel, Logging)
// 	logAll.Info("Starting Sensor Hub...")

// 	setupViaHotspot()

// 	// logAll.Info("Checking for existing Wi-Fi connection...")
// 	// if isConnected() {
// 	// 	logAll.Info("Already connected to Wi-Fi. No setup needed.")
// 	// 	availableNetworksHTML, err := getAvailableNetworksHTML()
// 	// 	// availableNetworks, err := getAvailableNetworks()
// 	// 	if err != nil {
// 	// 		logAll.Error("Failed to get available Wi-Fi networks", "error", err)
// 	// 	} else {
// 	// 		logAll.Info(fmt.Sprintf("Available Wi-Fi networks:\n%v\n", availableNetworksHTML))
// 	// 	}
// 	// 	return
// 	// }

// 	// logAll.Info("No existing connection detected. Starting setup hotspot and server...")
// 	// availableNetworks, err := getAvailableNetworks()
// 	// if err != nil {
// 	// 	logAll.Error("Failed to get available Wi-Fi networks", "error", err)
// 	// } else {
// 	// 	logAll.Info(fmt.Sprintf("Available Wi-Fi networks:\n%s\n", string(availableNetworks)))
// 	// }

// 	// http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
// 	// 	t, _ := template.New("webpage").Parse(formHTML)
// 	// 	t.Execute(w, nil)
// 	// })

// 	// http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
// 	// 	if r.Method != http.MethodPost {
// 	// 		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
// 	// 		return
// 	// 	}

// 	// 	ssid := r.FormValue("ssid")
// 	// 	password := r.FormValue("password")

// 	// 	fmt.Fprintf(w, "<h1>Trying to connect to %s...</h1><p>The Sensor Hub setup hotspot will reboot. If new wifi connection fails, the hotspot will return in roughly 30 seconds.</p>", ssid)

// 	// 	if flusher, ok := w.(http.Flusher); ok {
// 	// 		flusher.Flush()
// 	// 	}

// 	// 	// Handle network switching asynchronously
// 	// 	go attemptConnection(ssid, password)
// 	// })

// 	// logAll.Info(fmt.Sprintf("Sensor Hub Wi-Fi setup server listening on port %s...", serverPort))
// 	// if err := http.ListenAndServe(serverPort, nil); err != nil {
// 	// 	logAll.Error("Server exited with error", "error", err)
// 	// }
// }

func attemptConnection(ssid, password string) {
	logAll.Info(fmt.Sprintf("Initiating connection to target SSID: %s\n", ssid))

	// 1. Request NetworkManager to connect to target network
	// nmcli automatically handles turning off the current AP when joining a client network
	cmd := exec.Command("nmcli", "device", "wifi", "connect", ssid, "password", password)
	output, err := cmd.CombinedOutput()

	if err != nil {
		logAll.Error(fmt.Sprintf("Connection failed: %s. Initiating fallback mechanism.\n", string(output)))
		recoverHotspot()
		return
	}

	// 2. Double-check connectivity by verifying IP assignment
	logAll.Info(fmt.Sprintln("nmcli reported success. Verifying network route..."))
	time.Sleep(5 * time.Second) // Give DHCP time to assign an IP address

	if isConnected() {
		logAll.Info(fmt.Sprintln("Successfully online! System is permanently connected."))
		return
	}

	logAll.Info(fmt.Sprintln("Successfully associated with AP but failed to fetch an IP. Returning to hub hotspot."))
	recoverHotspot()
}

func recoverHotspot() {
	logAll.Info(fmt.Sprintln("Restoring local setup hotspot..."))

	// Reactivate the pre-configured NetworkManager hotspot profile
	cmd := exec.Command("nmcli", "connection", "up", hotspotProfile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logAll.Error(fmt.Sprintf("Critical Error: Failed to restart hotspot profile: %s\n", string(output)))
		return
	}

	logAll.Info(fmt.Sprintln("Setup hotspot is online again. Awaiting user retry."))
}

func isConnected() bool {
	// Quick check to see if the device can route packets to the internet
	cmd := exec.Command("ping", "-c", "1", "-W", "2", "8.8.8.8")
	err := cmd.Run()
	return err == nil
}

func configured() bool {
	// Check if the device is already configured to connect to a Wi-Fi network
	// We use a text file to hold the SSID of the currently configured network. If it exists, we assume we're configured.
	// Filename is /var/sensorhub/sensorhub.config and contains the SSID of the currently configured network.
	// If the file doesn't exist, we assume we're not configured.
	_, err := os.Stat(pathToHubConfig)
	if err == nil {
		return true // file exists
	}
	// if os.IsNotExist(err) {
	// 	f, err := os.OpenFile(pathToHubConfig, os.O_CREATE|os.O_EXCL, 0644)
	// 	if err != nil {
	// 		// file might have been created concurrently or another error occurred
	// 		if os.IsExist(err) {
	// 			return true
	// 		}
	// 		return false
	// 	}
	// 	_ = f.Close()
	// 	return false // file created
	// }
	return false // No config file, return false
}

func initConfig(ssid string, path string) error {
	// Write the SSID to the config file for persistence
	// Overwrites any existing content, which is fine since we only need to store the current SSID
	// Creates the file if it doesn't exist, or truncates it if it does. Permissions are set to 0644 (read/write for owner, read for group and others).
	return os.WriteFile(path, []byte(ssid), 0644)
}
