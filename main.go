package main

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os/exec"
	"time"
)

const (
	Logging  = true            // Always on for logging. Set level with LogLevel property
	LogLevel = slog.LevelDebug // Change to Error for production
)

const (
	hotspotProfile = "HubHotspot" // The nmcli profile name for your Pi setup AP
	serverPort     = ":80"
)

const formHTML = `
<!DOCTYPE html>
<html>
<head>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Pi Wi-Fi Setup</title>
</head>
<body>
    <h2>Connect Sensor Hub to WiFi</h2>
    <form action="/connect" method="POST">
        <label>SSID (Network Name):</label><br>
        <input type="text" name="ssid" required><br><br>
        <label>Password:</label><br>
        <input type="password" name="password" required><br><br>
        <input type="submit" value="Connect">
    </form>
</body>
</html>`

var logAll *slog.Logger

func main() {
	logAll = ConfigureLogging(LogLevel, Logging)
	logAll.Info("Starting Sensor Hub Wi-Fi setup server...")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, _ := template.New("webpage").Parse(formHTML)
		t.Execute(w, nil)
	})

	http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ssid := r.FormValue("ssid")
		password := r.FormValue("password")

		fmt.Fprintf(w, "<h1>Trying to connect to %s...</h1><p>The Sensor Hub setup hotspot will reboot. If new wifi connection fails, the hotspot will return in roughly 30 seconds.</p>", ssid)

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		// Handle network switching asynchronously
		go attemptConnection(ssid, password)
	})

	logAll.Info(fmt.Sprintf("Sensor Hub Wi-Fi setup server listening on port %s...", serverPort))
	if err := http.ListenAndServe(serverPort, nil); err != nil {
		logAll.Error("Server exited with error", "error", err)
	}
}

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
