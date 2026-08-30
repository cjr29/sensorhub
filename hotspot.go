package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	HotspotConnName = "SensorHubHotspot" // The nmcli profile name for your Pi setup AP
	HotspotSSID     = "SensorHub"
	HotspotPWD      = "ConfigureMe123" // Minimum 8 characters
	WiFiConnName    = "wifi-home"
	// sudo nmcli device wifi hotspot ifname wlan0 con-name "SensorHubHotspot" ssid SensorHub password ConfigureMe123

	listenAddr     = ":8080"
	refreshSeconds = 480 // Gives user 4 minutes to enter SSID and password
	nmcliTimeout   = 5 * time.Second

	pathToHubConfig = "/var/sensorhub/sensorhub.conf"
)

type WiFi struct {
	SSID string `json:"ssid"`
	PWD  string `json:"pwd"`
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

var pageTmpl = template.Must(template.ParseFiles("/usr/local/bin/sensorhub/templates/showwifi.html"))

var currentWiFi = WiFi{}

// loginWiFiViaHotspot starts a temporary hotspot,
// launches the restservice to monitor for configuration request,
// collects selected WiFi SSID and password,
// and then connects to the specified WiFi network.
// If the connection fails, it will loop back to the hotspot configuration step.
func runWiFiViaHotspot() {
	logger.Debug("Entered runWiFiViaHotspot.")
	if err := startHotspot(); err != nil {
		logger.Error(fmt.Sprintf("Failed to start hotspot: %v", err))
		// Generate signal to stop Sensor Hub
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}

	// Create a channel to catch the WiFi login credentials once submitted
	// from the restservice
	// Return from the channel will be a [2]string array with SSID and PWD
	credChan := make(chan [2]string)

	server := startWebServer(credChan)

	// Wait until the user sends credentials via the captive portal web interface
	logger.Debug("Waiting for user to enter SSID and password")
	creds := <-credChan
	currentWiFi.SSID = creds[0]
	currentWiFi.PWD = creds[1]

	logger.Debug(fmt.Sprintf("Received credentials for SSID: %s. \n Shutting down web server to connect to WiFi.", currentWiFi.SSID))
	// Persist WiFi credentials
	currentWiFi.saveSSIDPWD()

	// Gracefully shut down web server so the HTTP response finishes flushing to the browser
	_ = server.Shutdown(context.Background())

	logger.Debug("Stopping hotspot.")
	_ = stopHotspot()

	// Use credentials just received to recreate a clean wifi-home connection profile
	// Add the new connection profile before trying to bring it up
	// Step 1: Delete existing profile if it exisits
	if connectionExists(WiFiConnName) {
		deleteConnection(WiFiConnName)
	}
	err := addWifiConnection(WiFiConnName, currentWiFi.SSID, currentWiFi.PWD)
	if err != nil {
		// Could not connect to WiFi, loop until success
		loginWiFiViaHotspot()
	}
	// WiFi Connected
	logger.Info(fmt.Sprintf("Successfully connected to the new Wi-Fi network %s", currentWiFi.SSID))
	lightCMD <- GREENON
}

func loginWiFiViaHotspot() {
	logger.Debug("Entered loginWiFiViaHotspot.")
	if err := startHotspot(); err != nil {
		logger.Error(fmt.Sprintf("Failed to start hotspot: %v", err))
		// Generate signal to stop Sensor Hub
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}

	// Create a channel to catch the WiFi login credentials once submitted
	credChan := make(chan [2]string)
	server := startWebServer(credChan)

	// Wait until the user sends credentials via the captive portal web interface
	logger.Debug("Waiting for user to enter SSID and password")
	creds := <-credChan
	currentWiFi.SSID = creds[0]
	currentWiFi.PWD = creds[1]

	logger.Debug(fmt.Sprintf("Received credentials for SSID: %s. \n Shutting down web server to connect to WiFi.", currentWiFi.SSID))
	// Persist WiFi credentials
	currentWiFi.saveSSIDPWD()

	// Gracefully shut down web server so the HTTP response finishes flushing to the browser
	_ = server.Shutdown(context.Background())

	logger.Debug("Stopping hotspot.")
	_ = stopHotspot()

	// Use credentials just received to recreate a clean wifi-home connection profile
	// Add the new connection profile before trying to bring it up
	// Step 1: Delete existing profile if it exisits
	if connectionExists(WiFiConnName) {
		deleteConnection(WiFiConnName)
	}
	err := addWifiConnection(WiFiConnName, currentWiFi.SSID, currentWiFi.PWD)
	if err != nil {
		// Could not connect to WiFi, loop until success
		loginWiFiViaHotspot()
	}
	// WiFi Connected
	logger.Info(fmt.Sprintf("Successfully connected to the new Wi-Fi network %s", currentWiFi.SSID))
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
		// sudo nmcli device wifi hotspot ifname wlan0 con-name SensorHubHotspot ssid SensorHub password ConfigureMe123
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
	logger.Debug("Entered stopHotspot")
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
	// Step 1: Delete existing profile if it exisits
	if connectionExists(WiFiConnName) {
		deleteConnection(WiFiConnName)
	} // Step 2: Add new connection profile
	err := addWifiConnection(WiFiConnName, ssid, pwd)
	if err != nil {
		// Could not connect to WiFi
		return fmt.Errorf("Failed to connect to %s: %v", ssid, err)
	}
	// WiFi Connected
	logger.Info(fmt.Sprintf("Successfully connected to the WiFi network %s", ssid))

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
	logger.Debug("Connected to wifi now. Persisting SSID and PWD.")
	if err := currentWiFi.saveSSIDPWD(); err == nil {
		logger.Debug(fmt.Sprintf("WiFi SSID and PWD saved to %s", pathToHubConfig))
	} else {
		logger.Error(fmt.Sprintf("Unable to save SSID and password to config file. Error: %s", err))
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

// Launches a temporary lightweight Go HTTP web server
func startWebServer(credChan chan [2]string) *http.Server {
	logger.Debug("Entered startWebServer")
	mux := http.NewServeMux()
	server := &http.Server{Addr: listenAddr, Handler: mux}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// tmpl, _ := template.New("form").Parse(inputformHTML)
		// _ = tmpl.Execute(w, nil)
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
			time.Sleep(5 * time.Second)
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

func (w *WiFi) getSSIDPWD() (bool, error) {
	// Read JSON from file
	jsonData, err := os.ReadFile(pathToHubConfig)
	if err != nil {
		logger.Error("Unable to read the configuration file")
		return false, err
	}
	// Convert JSON to struct
	err = json.Unmarshal(jsonData, w)
	if err != nil {
		logger.Error("Unable to convert JSON to WiFi struct")
		return false, err
	}
	// Successful. SSID and PWD are in the WiFi struct w
	return true, nil
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

// Checks to see if WiFi has already been configured, and if so, restarts the connection
func wifiConfigured() bool {
	logger.Debug("Entering wifiConfigured.")
	// Does config file exist?
	_, err := currentWiFi.getSSIDPWD()
	if err != nil {
		// WiFi not configured
		logger.Debug("Unable to find or read config file. WiFi not configured.")
		wifi = false
		return false
	} else {
		// // Has an SSID and PWD. Try opening a connection.
		// err = connectToWiFi(WiFiConnName, currentWiFi.SSID, currentWiFi.PWD)
		// if err == nil {
		// 	// WiFi up
		wifi = true
		logger.Debug(fmt.Sprintf("WiFi preconfigured for SSID %s", currentWiFi.SSID))
		// 	return true
		// } else {
		// 	// Unable to reopen wifi using existing configuration
		// 	logger.Debug(fmt.Sprintf("Unable to open WiFi with existing configuration for SSID %s", currentWiFi.SSID))
		// 	return false
		// }
		return true
	}
}

/******************************************************/
/* Unused functions - For future enhancements *********/
/******************************************************/

func profileExists(ctx context.Context, connName string) (bool, error) {
	// Use nmcli in terse, script-friendly mode.
	// -f NAME shows only connection names
	// -t (terse) removes headers/formatting.
	// nmcli will exit 0 even if no connections exist; we just check output.
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, "nmcli",
		"-t", "-f", "NAME", "connection", "show")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If nmcli isn't installed or NetworkManager isn't available, surface error.
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return false, fmt.Errorf("nmcli failed: %w (%s)", err, msg)
		}
		return false, fmt.Errorf("nmcli failed: %w", err)
	}

	target := strings.TrimSpace(connName)
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.TrimSpace(line) == target {
			return true, nil
		}
	}
	return false, nil
}

// ConnectionActive checks whether a given NetworkManager connection exists and is active.
// It returns (true, nil) if the connection is active, otherwise (false, nil).
func connectionActive(ctx context.Context, connName string) (bool, error) {
	if strings.TrimSpace(connName) == "" {
		return false, fmt.Errorf("connName is empty")
	}
	// Check connection exists:
	// nmcli -t -f NAME con show
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
	// nmcli -t -f NAME,TYPE,DEVICE con show --active
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
