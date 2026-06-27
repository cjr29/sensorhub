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
	HotspotPass     = "ConfigureMe123" // Minimum 8 characters
	WiFiConnName    = "wifi-home"
	// sudo nmcli device wifi hotspot ifname wlan0 con-name "SensorHubHotspot" ssid SensorHub password ConfigureMe123

	listenAddr     = ":8080"
	refreshSeconds = 120
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

func loginWifiViaHotspot() {
	logger.Debug("Entered loginWifiViaHotspot.")
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

	// Use creditials just received to recreate a clean wifi-home connection profile
	// Add the new connection profile before trying to bring it up
	// sudo nmcli connection add type wifi con-name HomeWiFi ifname wlan0 ssid MyNetwork wifi-sec.key-mgmt wpa-psk wifi-sec.psk mysecretpassword
	logger.Debug(fmt.Sprintf("Adding new WiFi connection profile for %s", WiFiConnName))
	cmd := exec.Command("sudo", "nmcli", "connection", "add",
		"type", "wifi",
		"con-name", WiFiConnName,
		"ifname", "wlan0",
		"ssid", currentWiFi.SSID,
		"wifi-sec.key-mgmt", "wpa-psk",
		"wifi-sec.psk", currentWiFi.PWD,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fatal error, quit program
		logger.Error(fmt.Sprintf("Failed to create wifi connection profile: %s", output))
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}

	logger.Debug(fmt.Sprintf("Connecting to WiFi network '%s'...", currentWiFi.SSID))
	if err := connectToWiFi(WiFiConnName, currentWiFi.SSID, currentWiFi.PWD); err != nil {
		logger.Error(fmt.Sprintf("Failed to connect: %v. Re-triggering hotspot loop...", err))
		// Option: Call main() again or loop to safely recover the hotspot state
		// Generate signal to stop Sensor Hub
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}
	logger.Info(fmt.Sprintf("Successfully connected to the new Wi-Fi network %s", currentWiFi.SSID))
}

// Configures and enables the Pi's native hotspot using nmcli
func startHotspot() error {
	logger.Debug("Entered startHotspot")

	// First be sure to shut down any Wifi connection before starting the hotspot
	exists, err := connectionExists(context.Background(), WiFiConnName)
	if exists {
		logger.Debug(fmt.Sprintf("%s wifi profile exists, check if active.", WiFiConnName))
		active, err := connectionActive(context.Background(), WiFiConnName)
		if active {
			logger.Debug(fmt.Sprintf("%s wifi connection is active", WiFiConnName))
			// WiFi needs to be down before we can run the hotspot
			err = stopWifi(WiFiConnName)
			if err != nil {
				logger.Error("Failed to stop wifi. Cannot proceeed with setup")
				return (err)
			}
			logger.Debug(fmt.Sprintf("%s WiFi connection has been brought down.", WiFiConnName))
		} else {
			logger.Debug(fmt.Sprintf("%s wifi connection is not active", WiFiConnName))
		}
	}

	// Be sure that the hotspot connection profile exists, create it if it does not exist
	exists, err = connectionExists(context.Background(), HotspotConnName)
	if err != nil {
		return (err)
	}
	if exists {
		// If hotspot profile already exists, check if active and if it is , stop it
		active, err := connectionActive(context.Background(), HotspotConnName)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed checking hotspot connection. %s", err))
			return (err)
		}
		if active {
			// Connection is active, return no error. Hotspot already up and running.
			return nil
		} else {
			// Exists, but not active, bring it up
			logger.Debug("SensorHubHotspot not active. Bring it up.")
			return exec.Command("sudo", "nmcli", "connection", "up", HotspotConnName).Run()
		}
	} else {
		logger.Debug("SensorHubHotspot profile does not exist. Create it.")
		// Create the new hotspot network profile and start it
		// sudo nmcli device wifi hotspot ifname wlan0 con-name SensorHubHotspot ssid SensorHub password ConfigureMe123
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
		// Bring hotspot up
		err := exec.Command("sudo", "nmcli", "connection", "down", HotspotConnName).Run()
		if err != nil {
			logger.Error(fmt.Sprintf("Failed bringing up hotspot connection %s", HotspotConnName))
		}
		logger.Debug(fmt.Sprintf("Hotspot connection %s for SSID %s is up.", HotspotConnName, HotspotSSID))
	}
	return nil
}

// // IsConnectionActive returns true if `name` is currently active in NetworkManager.
// func isConnectionActive(name string) (bool, error) {
// 	// nmcli -t -f NAME,DEVICE connection show --active
// 	// Output lines like: "MyVPN:eth0"
// 	cmd := exec.Command("nmcli", "-t", "-f", "NAME,DEVICE", "connection", "show", "--active")
// 	var out bytes.Buffer
// 	cmd.Stdout = &out
// 	cmd.Stderr = &out

// 	if err := cmd.Run(); err != nil {
// 		return false, fmt.Errorf("nmcli failed: %v (%s)", err, strings.TrimSpace(out.String()))
// 	}

// 	want := name + ":"
// 	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
// 		if strings.HasPrefix(line, want) {
// 			return true, nil
// 		}
// 	}
// 	return false, nil
// }

// Shuts down the temporary hotspot network
func stopHotspot() error {
	logger.Debug("Entered stopHotspot")
	logger.Debug(fmt.Sprintf("Shut down hotspot with connection name %s", HotspotConnName))
	return exec.Command("sudo", "nmcli", "connection", "down", HotspotConnName).Run()
}

// Shuts down the wifi network
func stopWifi(wifiProfile string) error {
	logger.Debug("Entered stopWifi")
	logger.Debug(fmt.Sprintf("Shutting down previous WiFi connection %s if it exists.", wifiProfile))
	// Be sure there is a wifi connection profile before trying to down it!
	exists, err := profileExists(wifiProfile)
	if err != nil {
		logger.Debug(fmt.Sprintf("Failed check for wifi connection profile. %s", err))
		return err
	}
	if exists {
		logger.Debug(fmt.Sprintf("Shut down wifi with connection name %s", wifiProfile))
		return exec.Command("sudo", "nmcli", "connection", "down", wifiProfile).Run()
	} else {
		logger.Debug("No WiFi connection available. No need to shut down.")
		return nil
	}
}

// uses nmcli to delete specified connection profile
func removeConnection(name string) error {
	logger.Debug("Entered removeConnection")
	var cmd *exec.Cmd
	cmd = exec.Command("sudo", "nmcli", "connection", "delete", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

// Commands NetworkManager to join the network specified by ssid, pwd
func connectToWiFi(connectionName, ssid string, pwd string) error {
	logger.Debug("Entered connectToWiFi")
	var cmd *exec.Cmd
	// Remove old wifi profile and connection
	err := stopWifi(connectionName)
	if err != nil {
		logger.Debug(fmt.Sprintf("stopWifi failed with error: %s", err))
		panic(err)
	}
	cmd = exec.Command("sudo", "nmcli", "connection", "down", connectionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("Failed to shutdown previous wifi connection.")
		return fmt.Errorf("%v: %s", err, string(output))
	}
	logger.Debug(fmt.Sprintf("Deleting previous WiFi connection %s if it exists.", connectionName))
	err = removeConnection(connectionName)
	if err != nil {
		logger.Error("Failed to remove exiting wifi connection profile.")
		return fmt.Errorf("%v: %s", err, string(output))
	}
	logger.Debug(fmt.Sprintf("Bring up WiFi connection %s", connectionName))
	cmd = exec.Command("sudo", "nmcli", "connection", "up", connectionName)
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

// ProfileExists returns true if nmcli has a connection profile with the given name.
func profileExists(name string) (bool, error) {
	// Use nmcli to list connection names and check for an exact match.
	logger.Debug(fmt.Sprintf("Entering profileExists to check existence of %s", name))
	cmd := exec.Command("nmcli", "-t", "-f", "name", "connection", "show")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}
	return false, nil
}

func connectionExists(ctx context.Context, connName string) (bool, error) {
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
	activeCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME,TYPE,DEVICE", "con", "show", "--active")
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

// Check for live connectivity by pinging an public DNS server
func hasInternet() bool {
	cmd := exec.Command("ping", "-c", "1", "-W", "2", "8.8.8.8")
	return cmd.Run() == nil
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
