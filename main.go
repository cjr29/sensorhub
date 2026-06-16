package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
)

const (
	Logging  = true            // Always on for logging. Set level with LogLevel property
	LogLevel = slog.LevelDebug // Change to Error for production
)

const (
	logDir          = "/var/log/sensorhub"
	logFile         = "/var/log/sensorhub/sensorhub.log"
	pidFile         = "/run/sensorhub/sensorhub.pid"
	hotspotProfile  = "HubHotspot" // The nmcli profile name for your Pi setup AP
	serverPort      = ":80"
	pathToHubConfig = "/var/sensorhub/sensorhub.conf"
)

type fileHandler struct {
	mu   sync.RWMutex
	file *os.File
	h    slog.Handler
}

func newFileHandler(path string) (*fileHandler, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	h := slog.NewTextHandler(f, &slog.HandlerOptions{
		Level:       LogLevel,
		AddSource:   false,
		ReplaceAttr: nil,
	})
	return &fileHandler{file: f, h: h}, nil
}

func (fh *fileHandler) Handle(ctx context.Context, r slog.Record) error {
	fh.mu.RLock()
	h := fh.h
	fh.mu.RUnlock()
	return h.Handle(ctx, r)
}

func (fh *fileHandler) Enabled(ctx context.Context, l slog.Level) bool {
	fh.mu.RLock()
	defer fh.mu.RUnlock()
	return fh.h.Enabled(ctx, l)
}

func (fh *fileHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	fh.mu.RLock()
	h := fh.h.WithAttrs(attrs)
	fh.mu.RUnlock()
	return &fileHandler{file: fh.file, h: h}
}

func (fh *fileHandler) WithGroup(name string) slog.Handler {
	fh.mu.RLock()
	h := fh.h.WithGroup(name)
	fh.mu.RUnlock()
	return &fileHandler{file: fh.file, h: h}
}

// Reopen closes the current file and opens a new one at the same path, swapping the handler.
func (fh *fileHandler) Reopen(path string) error {
	newf, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	newh := slog.NewTextHandler(newf, &slog.HandlerOptions{
		Level:       LogLevel,
		AddSource:   false,
		ReplaceAttr: nil,
	})

	fh.mu.Lock()
	oldf := fh.file
	fh.file = newf
	fh.h = newh
	fh.mu.Unlock()

	if oldf != nil {
		_ = oldf.Close()
	}
	return nil
}

func writePID(pidPath string) error {
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(pidPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%d\n", os.Getpid())
	return err
}

func removePID(pidPath string) {
	_ = os.Remove(pidPath)
}

func ensureLogDir() error {
	return os.MkdirAll(logDir, 0o755)
}

var logAll *slog.Logger
var logger *slog.Logger

var tmpl = template.Must(template.New("").ParseGlob("templates/*.html"))

func main() {

	// logAll = ConfigureLogging(LogLevel, Logging)
	// logAll.Info("Starting Sensor Hub...")

	if err := ensureLogDir(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log dir: %v\n", err)
		os.Exit(1)
	}

	fh, err := newFileHandler(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file: %v\n", err)
		os.Exit(1)
	}
	logger = slog.New(fh)

	if err := writePID(pidFile); err != nil {
		logger.Error("failed to write pidfile", "err", err)
		// continue; not fatal
	}
	defer removePID(pidFile)

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
					logger.Error("failed to reopen log file", "err", err)
				} else {
					logger.Info("reopened log file after SIGHUP")
				}
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("shutting down", "signal", sig.String())
				cancel()
				return
			}
		}
	}()

	// Example work loop: write a log message every 5s
	// ticker := time.NewTicker(5 * time.Second)
	// defer ticker.Stop()

	logger.Info("Sensor Hub service starting")
	for {
		select {
		case <-ctx.Done():
			logger.Info("service stopped")
			return
		// case t := <-ticker.C:
		// 	logger.Info("heartbeat", "time", t.Format(time.RFC3339))
		// }
		default:
			// Check if sensor hub is already configured to connect to a Wi-Fi network. If not, start the hotspot and web server for setup.
			if configured() {
				logger.Info("Already configured to connect to Wi-Fi. Now test connection.")
				if isConnected() {
					logger.Info("Already connected to Wi-Fi. No setup needed.")
				} else {
					logger.Info("Configured Wi-Fi network not currently reachable. Starting hotspot configuration.")
					configureViaHotspot()
				}
			} else {
				logger.Info("WiFi has not been configured. Starting hotspot configuration.")
				configureViaHotspot()
			}
		}
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
		logger.Info("Configuration file found at " + pathToHubConfig)
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

// func initConfig(ssid string, path string) error {
// 	// Write the SSID to the config file for persistence
// 	// Overwrites any existing content, which is fine since we only need to store the current SSID
// 	// Creates the file if it doesn't exist, or truncates it if it does. Permissions are set to 0644 (read/write for owner, read for group and others).
// 	return os.WriteFile(path, []byte(ssid), 0644)
// }
