package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"periph.io/x/host/v3"
)

const AppVersion = "0.6.1"

// Uses mDNS address to identify the sensor hub.
// This allows multiple sensor hubs to be used on the same network without conflict.
// The hostname is set to "sensorhub" + 12 hex digits of MAC address of wlan0 without the colons.

var hostName string
var busConfigured bool = false

func main() {
	// Create a context that will be canceled when SIGTERM signal is received
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	ctxLED, stopLED := context.WithCancel(context.Background())
	defer stop()
	defer stopLED()
	var wg sync.WaitGroup

	configureLogging()

	logger.Info("Starting Sensor Hub main program. Version: " + AppVersion)

	// Initialize the host GPIO driver to support LED and pushbutton.
	if !busConfigured {
		if _, err := host.Init(); err != nil {
			logger.Error(fmt.Sprintf("Error initializing GPIO: %v\n", err))
			os.Exit(1)
		}
		busConfigured = true
	}

	// Start activity LED
	ledControl := make(chan int)
	logger.Debug("Starting LED background thread.")
	go blinkActivityLED(ctxLED, ledControl)

	// Wait for the LED to be initialized before sending commands to it
	time.Sleep(time.Duration(1) * time.Second)
	logger.Debug("Sending initial LED command to set LED to red slow blink to indicate startup.")
	ledControl <- REDSLOW // Set LED to red slow blink to indicate startup

	// Retrieve hostname and log it
	var err error
	hostName, err = getHostname()
	if err != nil {
		logger.Error("Failed to retrieve hostname", "err", err)
	} else {
		logger.Info(fmt.Sprintf("Sensor Hub Hostname: %s", hostName))
	}

	// Set hostname to sensorhub + 12 hex digits of MAC address of wlan0 without the colons
	// This is done to avoid conflicts when multiple sensor hubs are on the same network.
	if err := EnsureSensorHubHostname(); err != nil {
		ledControl <- REDFAST // Set LED to red fast blink to indicate error
		logger.Error("Failed to set hostname", "err", err)
	} else {
		hostName, err = getHostname()
		if err != nil {
			ledControl <- REDFAST // Set LED to red fast blink to indicate error
			logger.Error("Failed to retrieve hostname", "err", err)
		} else {
			logger.Info(fmt.Sprintf("Sensor Hub Hostname set to: %s", hostName))
		}
	}

	// Check if sensor hub is already connected to a Wi-Fi network. If not, start the hotspot and web server for setup.
	// Existence of the conf file is evidence that configuration has occurred. Then, need to test connection.
	logger.Info("Verify that weather.db exists and is writable. If not, create it.")
	db, err = initDB(dbPath)
	// db, err = OpenOrCreateSQLite(dbPath)
	if err != nil {
		logger.Error(fmt.Sprintf("DB init failed: %v", err))
	}
	defer db.Close()

	// Configure background processing channels
	wg.Add(4)
	defer wg.Done()

	logger.Info("Check if Sensor Hub WiFi has been configured.")
	if !wifiConfigured() {
		// WiFi not configured. Start Hotspot and wait for restservice to handle configuration.
		ledControl <- GREENFAST // Set LED to green fast blink to indicate waiting for WiFi configuration
		if err := startHotspot(); err != nil {
			ledControl <- REDFAST // Set LED to red fast blink to indicate error
			logger.Error(fmt.Sprintf("Failed to start hotspot: %v", err))
		} else {
			ledControl <- GREENSLOW // Set LED to green slow blink to indicate hotspot started and waiting for configuration
			logger.Info(fmt.Sprintf("Hotspot started. Waiting for WiFi configuration via REST service on SSID %s", HotspotSSID))
		}
	} else {
		logger.Info(fmt.Sprintf("WiFi has been preconfigured. Using preset SSID %s", currentWiFi.SSID))
		ledControl <- GREENON // Set LED to green on to indicate WiFi is configured and connected
	}

	// Start the REST service in a separate goroutine
	go restService(ctx, &wg, logger)

	// logger.Info(fmt.Sprintf("WiFi connected to %s", currentWiFi.SSID))
	// logger.Debug("Pause 2 seconds for WiFi to stabilize before checking Internet access.")
	// time.Sleep(time.Duration(2) * time.Second)

	// See if Internet (remote) access is available
	if hasInternet() {
		ledControl <- GREENON // Set LED to green on to indicate WiFi is configured and connected
		logger.Info("Initializing: Internet access confirmed")
	} else {
		ledControl <- GREENSLOW // Set LED to green slow to indicate WiFi is configured but no Internet access
		logger.Info("Initializing: Only localhost access available. No Internet.")
	}

	/************************************************************************************/
	/********  WiFi Configured, proceed with processing *********************************/

	// Start the background processing threads now that we are connected to WiFi
	logger.Info(">>>>> Starting remaining background processes")

	go processSensorData(ctx, &wg, logger)       // Background thread to process sensor data
	go removeExpiredSensors(ctx, &wg, logger)    // Background thread to process expired sensors
	go monitorWiFi(ctx, &wg, ledControl, logger) // Background thread to monitor Wi-Fi connection and handle setup if needed

	// Handle other signals while waiting for goroutines to halt.
	stopChannel := make(chan os.Signal, 1)
	signal.Notify(stopChannel,
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGHUP,
		// add more signals you care about
	)
	defer signal.Stop(stopChannel)

	// Main program loop watching for system control signals and either reopening logs or terminating processing
	for {
		sig := <-stopChannel
		switch sig {
		case syscall.SIGTERM:
			// SIGTERM is handled by the context cancellation above, so just log and continue.
			logger.Debug("SIGTERM received by main thread; waiting for goroutines to exit.")
			stopLED()
			wg.Wait()
			logger.Info("!!!!! All background processing halted. Exiting SensorHub.")
		case syscall.SIGINT:
			logger.Debug("SIGINT received by main thread; waiting for goroutines to exit")
			stopLED()
			wg.Wait()
			logger.Info("!!!!! All background processing halted. Exiting SensorHub.")
		case syscall.SIGHUP:
			// watch for log rotations and reopen log
			logger.Debug("SIGHUP received by main thread.")
			// reopen log file after logrotate signal received
			if err := logFh.Reopen(logFile); err != nil {
				logger.Error("Failed to reopen log file", "err", err)
			} else {
				logger.Info("Reopened log file after SIGHUP")
			}
		}
	}
}
