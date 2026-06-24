package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	// logger = ConfigureLogging(LogLevel, Logging)
	if err := ensureLogDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log dir: %v\n", err)
		os.Exit(1)
	}
	fh, err := newFileHandler(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	logger = slog.New(fh)
	logger.Info(fmt.Sprintf("Logging configured for %s.", LogLevel.String()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// signals: SIGINT/SIGTERM -> shutdown, SIGHUP -> reopen log
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Watches for system signals in a separate goroutine so it doesn't block the main work loop
	go func() {
		for {
			sig := <-sigCh
			switch sig {
			case syscall.SIGHUP:
				// reopen logs for logrotate
				if err := fh.Reopen(logFile); err != nil {
					logger.Error("Failed to reopen log file", "err", err)
				} else {
					logger.Info("Reopened log file after SIGHUP")
				}
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info(fmt.Sprintf("Shutting down: %s signal received ...", sig.String()))
				cancel()
				return
			}
		}
	}()

	// Check if sensor hub is already configured to connect to a Wi-Fi network. If not, start the hotspot and web server for setup.
	if isConfigFile() {
		if err := currentWiFi.getSSIDPWD(); err == nil {
			logger.Info(fmt.Sprintf("Already configured to connect to Wi-Fi SSID:%s. Now test connection.", currentWiFi.SSID))
			if isConnected() {
				logger.Info(fmt.Sprintf("WiFi on and connected to %s.", currentWiFi.SSID))
			} else {
				logger.Info(fmt.Sprintf("Configured Wi-Fi network %s not currently reachable. Starting hotspot configuration.", currentWiFi.SSID))
				configureViaHotspot()
			}
		} else {
			logger.Info("WiFi has not been configured. Starting hotspot configuration.")
			configureViaHotspot()
			logger.Info(fmt.Sprintf("WiFi has been connected to %s", currentWiFi.SSID))
		}
	}

	// Start the background processing threads now that we are connected to WiFi
	go processSensorData(ctx)    // Background thread to process sensor data
	go restService(ctx)          // Background thread to log that the REST service is still running
	go removeExpiredSensors(ctx) // Background thread to process expired sensors
	go monitorWiFi(ctx)          // Background thread to monitor Wi-Fi connection and handle setup if needed

	logger.Info("Sensor Hub setup complete.")

	// Wait for cancellation and exit
	<-ctx.Done()
	logger.Info("service stopped")
	os.Exit(0)
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

// func attemptConnection(ssid, password string) {
// 	logAll.Info(fmt.Sprintf("Initiating connection to target SSID: %s\n", ssid))

// 	// 1. Request NetworkManager to connect to target network
// 	// nmcli automatically handles turning off the current AP when joining a client network
// 	cmd := exec.Command("nmcli", "device", "wifi", "connect", ssid, "password", password)
// 	output, err := cmd.CombinedOutput()

// 	if err != nil {
// 		logAll.Error(fmt.Sprintf("Connection failed: %s. Initiating fallback mechanism.\n", string(output)))
// 		recoverHotspot()
// 		return
// 	}

// 	// 2. Double-check connectivity by verifying IP assignment
// 	logAll.Info(fmt.Sprintln("nmcli reported success. Verifying network route..."))
// 	time.Sleep(5 * time.Second) // Give DHCP time to assign an IP address

// 	if isConnected() {
// 		logAll.Info(fmt.Sprintln("Successfully online! System is permanently connected."))
// 		return
// 	}

// 	logAll.Info(fmt.Sprintln("Successfully associated with AP but failed to fetch an IP. Returning to hub hotspot."))
// 	recoverHotspot()
// }

// func recoverHotspot() {
// 	logAll.Info(fmt.Sprintln("Restoring local setup hotspot..."))

// 	// Reactivate the pre-configured NetworkManager hotspot profile
// 	cmd := exec.Command("nmcli", "connection", "up", hotspotProfile)
// 	output, err := cmd.CombinedOutput()
// 	if err != nil {
// 		logAll.Error(fmt.Sprintf("Critical Error: Failed to restart hotspot profile: %s\n", string(output)))
// 		return
// 	}

// 	logAll.Info(fmt.Sprintln("Setup hotspot is online again. Awaiting user retry."))
// }
