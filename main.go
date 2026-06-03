package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/exec"
	"time"
)

const (
	hotspotProfile = "Hotspot" // The nmcli profile name for your Pi setup AP
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
    <h2>Connect Raspberry Pi to Wi-Fi</h2>
    <form action="/connect" method="POST">
        <label>SSID (Network Name):</label><br>
        <input type="text" name="ssid" required><br><br>
        <label>Password:</label><br>
        <input type="password" name="password" required><br><br>
        <input type="submit" value="Connect">
    </form>
</body>
</html>`

func main() {
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

		fmt.Fprintf(w, "<h1>Trying to connect to %s...</h1><p>The Pi setup hotspot will turn off. If connection fails, the hotspot will return in roughly 30 seconds.</p>", ssid)

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		// Handle network switching asynchronously
		go attemptConnection(ssid, password)
	})

	log.Printf("Wi-Fi setup server listening on port %s...\n", serverPort)
	log.Fatal(http.ListenAndServe(serverPort, nil))
}

func attemptConnection(ssid, password string) {
	log.Printf("Initiating connection to target SSID: %s\n", ssid)

	// 1. Request NetworkManager to connect to target network
	// nmcli automatically handles turning off the current AP when joining a client network
	cmd := exec.Command("nmcli", "device", "wifi", "connect", ssid, "password", password)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("Connection failed: %s. Initiating fallback mechanism.\n", string(output))
		recoverHotspot()
		return
	}

	// 2. Double-check connectivity by verifying IP assignment
	log.Println("nmcli reported success. Verifying network route...")
	time.Sleep(5 * time.Second) // Give DHCP time to assign an IP address

	if isConnected() {
		log.Println("Successfully online! System is permanently connected.")
		return
	}

	log.Println("Successfully associated with AP but failed to fetch an IP. Rolling back.")
	recoverHotspot()
}

func recoverHotspot() {
	log.Println("Restoring local setup hotspot...")

	// Reactivate the pre-configured NetworkManager hotspot profile
	cmd := exec.Command("nmcli", "connection", "up", hotspotProfile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Critical Error: Failed to restart hotspot profile: %s\n", string(output))
		return
	}

	log.Println("Setup hotspot is online again. Awaiting user retry.")
}

func isConnected() bool {
	// Quick check to see if the device can route packets to the internet
	cmd := exec.Command("ping", "-c", "1", "-W", "2", "8.8.8.8")
	err := cmd.Run()
	return err == nil
}
