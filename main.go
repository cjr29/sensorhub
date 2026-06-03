package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/exec"
)

// HTML form template for credential entry
const formHTML = `
<!DOCTYPE html>
<html>
<head><title>Pi Wi-Fi Setup</title></head>
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

		fmt.Fprintf(w, "<h1>Connecting to %s...</h1><p>The Pi Access Point will disconnect shortly.</p>", ssid)

		// Flush response to user before taking down interface
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		// Run NetworkManager command asynchronously to prevent freezing the server
		go connectToWifi(ssid, password)
	})

	log.Println("Wi-Fi setup server listening on port 80...")
	log.Fatal(http.ListenAndServe(":80", nil))
}

func connectToWifi(ssid, password string) {
	log.Printf("Attempting connection to SSID: %s\n", ssid)

	// Execute NetworkManager CLI tool to connect to the target Wi-Fi network
	cmd := exec.Command("nmcli", "device", "wifi", "connect", ssid, "password", password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to connect: %s, Error: %v\n", string(output), err)
		return
	}

	log.Println("Successfully connected to the new Wi-Fi network!")
}
