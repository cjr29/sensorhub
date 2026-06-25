package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	pathToHubConfig = "/var/sensorhub/sensorhub.conf"
)

type WiFi struct {
	SSID string `json:"ssid"`
	PWD  string `json:"pwd"`
}

var tmpl = template.Must(template.New("").ParseGlob("/usr/local/bin/sensorhub/templates/*.html"))
var pageTmpl = template.Must(template.ParseFiles("/usr/local/bin/sensorhub/templates/showwifi.html"))

// var loginTmpl = template.Must(template.ParseFiles("/usr/local/bin/sensorhub/templates/login.html"))
var currentWiFi = WiFi{}

func configureViaHotspot() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/page1", listHandler)
	http.HandleFunc("/page2", loginHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("/usr/local/bin/sensorhub/static"))))

	logger.Debug("Entered configureViaHotspot")
	logger.Debug("Calling startHotspot()")
	if err := startHotspot(); err != nil {
		logger.Error(fmt.Sprintf("Failed to start hotspot: %v", err))
		// Generate signal to stop Sensor Hub
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.Error(err.Error())
	}
}

func render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		logger.Error("template execute error", "error", err.Error())
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "index.html", map[string]any{
		"Title": "Sensor Hub Setup",
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	// render(w, "page2.html", map[string]any{
	// 	"Title": "Login to WiFi Network",
	// 	"Body":  "Enter login information for selected WiFi network.",
	// })
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

func listHandler(w http.ResponseWriter, r *http.Request) {
	data := SSIDPageData{Refresh: refreshSeconds}

	// Use nmcli to list Wi-Fi networks in terse machine-friendly format
	// fields: SSID,SIGNAL,SECURITY, -p pretty, -t terse, -f fields
	out, err := runNmcli("-t", "-f", "SSID,SIGNAL,SECURITY", "device", "wifi", "list")
	if err != nil {
		data.Error = strings.TrimSpace(err.Error() + " — output: " + out)
		_ = pageTmpl.Execute(w, data)
		return
	}

	data.Networks = parseNmcliScan(out)
	if err := pageTmpl.Execute(w, data); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
	}
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

func (w *WiFi) saveSSIDPWD() error {
	// Write the SSID and PWD from struct w to the config file for persistence
	// Overwrites any existing content, which is fine since we only need to store the current SSID:PWD
	// Creates the file if it doesn't exist, or truncates it if it does. Permissions are set to 0644 (read/write for owner, read for group and others).
	_, err := os.Stat(pathToHubConfig)
	if os.IsNotExist(err) {
		f, err := os.OpenFile(pathToHubConfig, os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			logger.Error("Unable to create configuration file. Halting program.")
			return err
		}
		_ = f.Close()
	}
	// Convert to JSON
	jsonData, err := json.Marshal(w)
	if err != nil {
		logger.Error("Unable to convert SSID and password to JSON")
		return err
	}
	// Write JSON to file
	err = os.WriteFile(pathToHubConfig, jsonData, 0644)
	if err != nil {
		logger.Error("Unable to write SSID:PWD to config file")
		return err
	}
	// Successful write
	return nil
}

func (w *WiFi) getSSIDPWD() error {
	// Read JSON from file
	jsonData, err := os.ReadFile("message.json")
	if err != nil {
		logger.Error("Unable to read the configuration file")
		return err
	}
	// Convert JSON to struct
	err = json.Unmarshal(jsonData, w)
	if err != nil {
		logger.Error("Unable to convert JSON to WiFi struct")
		return err
	}
	// Successful. SSID and PWD are in the WiFi struct w
	return nil
}

// Check for existence of configuration file
// Check if the device is already configured to connect to a Wi-Fi network
// We use a json file to hold the SSID:PWD of the currently configured network. If it exists, we assume we're configured.
// Filename is /var/sensorhub/sensorhub.conf and contains the SSID and password of the currently configured network.
// If the file doesn't exist, we assume we're not configured.
func isConfigFile() bool {
	_, err := os.Stat(pathToHubConfig)
	if err == nil {
		return true // File exists
	}
	return false // No config file, return false
}
