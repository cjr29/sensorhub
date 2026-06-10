package main

import (
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
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

var logAll *slog.Logger

var tmpl = template.Must(template.New("").ParseGlob("templates/*.html"))

func main() {

	logAll = ConfigureLogging(LogLevel, Logging)
	logAll.Info("Starting Sensor Hub...")

	http.HandleFunc("/", indexHandler)
	// http.HandleFunc("/page1", page1Handler)
	http.HandleFunc("/page1", listHandler)
	http.HandleFunc("/page2", page2Handler)

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	log.Println("Listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

func render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Println("template execute error:", err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "index.html", map[string]any{
		"Title": "Sensor Hub Setup",
	})
}

func page1Handler(w http.ResponseWriter, r *http.Request) {
	render(w, "page1.html", map[string]any{
		"Title": "List of Available Networks",
		"Body":  "Displays available WiFi networks, refreshed every 5 seconds.",
	})
}

func page2Handler(w http.ResponseWriter, r *http.Request) {
	render(w, "page2.html", map[string]any{
		"Title": "Login to WiFi Network",
		"Body":  "Enter login information for selected WiFi network.",
	})
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	data := SSIDPageData{Refresh: refreshSeconds}

	// Use nmcli to list Wi-Fi networks in terse machine-friendly format
	// fields: SSID,BSSID,CHAN,RATE,SIGNAL,SECURITY
	out, err := runNmcli("-t", "-f", "SSID,BSSID,CHAN,RATE,SIGNAL,SECURITY", "device", "wifi", "list")
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
