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

const AppVersion = "0.6.7"

// Uses mDNS address to identify the sensor hub.
// This allows multiple sensor hubs to be used on the same network without conflict.
// The hostname is set to "sensorhub" + 12 hex digits of MAC address of wlan0 without the colons.

var hostName string
var busConfigured bool = false

// Channel for comman ding the light control goroutine to change the LED state
var lightCMD = make(chan int, 5)

func main() {
	// Create a context that will be canceled when SIGTERM signal is received
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	ctxLight, stopLight := context.WithCancel(context.Background())
	defer stop()
	defer stopLight()
	var wg sync.WaitGroup

	configureLogging()

	logger.Info("Starting Sensor Hub main program. Version: " + AppVersion)

	// Initialize the host GPIO driver to support LED and pushbutton.
	if !busConfigured {
		if _, err := host.Init(); err != nil {
			logger.Error(fmt.Sprintf("Error initializing GPIO: %v\n", err))
			os.Exit(1)
		}
		// Wait for the bus to be initialized before sending commands to it
		time.Sleep(time.Duration(1) * time.Second)
		busConfigured = true
	}
	logger.Debug("GPIO bus is configured")

	// Create activity Light and other layout status lights and button
	redLight = NewGPIOLED("Red Light", RED)
	greenLight = NewGPIOLED("Green Light", GREEN)

	logger.Debug("Starting LED background thread.")

	// // Start activity light in a goroutine
	go lightControl(ctxLight, lightCMD)

	logger.Debug("REDSLOW")
	lightCMD <- REDSLOW // Set LED to red slow blink to indicate startup
	time.Sleep(time.Duration(5) * time.Second)

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
		logger.Debug("REDFAST")
		lightCMD <- REDFAST // Set LED to red fast blink to indicate error
		logger.Error("Failed to set hostname. RED fast.", "err", err)
		time.Sleep(time.Duration(5) * time.Second)
		os.Exit(1)
	} else {
		hostName, err = getHostname()
		if err != nil {
			logger.Debug("REDFAST")
			lightCMD <- REDFAST // Set LED to red fast blink to indicate error
			logger.Error("Failed to retrieve hostname. RED fast.", "err", err)
			time.Sleep(time.Duration(5) * time.Second)
			os.Exit(1)
		} else {
			logger.Info(fmt.Sprintf("Sensor Hub Hostname set to: %s", hostName))
			lightCMD <- GREENFAST
		}
	}
	lightCMD <- GREENFAST // Set LED to green fast blink to indicate hostname set and ready for Wi-Fi configuration

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
		if err := startHotspot(); err != nil {
			lightCMD <- REDFAST // Set LED to red fast blink to indicate error
			logger.Error(fmt.Sprintf("Failed to start hotspot: %v", err))
			time.Sleep(time.Duration(5) * time.Second)
			os.Exit(1)
		} else {
			logger.Info(fmt.Sprintf("Hotspot started. Waiting for WiFi configuration via REST service on SSID %s", HotspotSSID))
		}
		// Hotspot is running. Waiting for user to configure using iOS app.
		logger.Debug("GREENFAST")
		lightCMD <- GREENFAST // Set LED to green fast blink to indicate waiting for WiFi configuration
	} else {
		logger.Info(fmt.Sprintf("WiFi has been preconfigured. Using preset SSID %s", currentWiFi.SSID))
		logger.Debug("GREENON")
		lightCMD <- GREENON // Set LED to green on to indicate WiFi is configured and connected
	}

	// Start the REST service in a separate goroutine
	// Starts now in order to handle Hotspot configuration if needed.
	// If WiFi is already configured, the REST service will still be available for other requests.
	go restService(ctx, &wg, logger)

	// logger.Info(fmt.Sprintf("WiFi connected to %s", currentWiFi.SSID))
	// logger.Debug("Pause 2 seconds for WiFi to stabilize before checking Internet access.")
	// time.Sleep(time.Duration(2) * time.Second)

	// See if Internet (remote) access is available
	if hasInternet() {
		logger.Debug("GREENON")
		lightCMD <- GREENON // Set LED to green on to indicate WiFi is configured and connected
		logger.Info("Initializing: Internet access confirmed")
	} else {
		logger.Debug("GREENSLOW")
		lightCMD <- GREENSLOW // Set LED to green slow to indicate WiFi is configured but no Internet access
		logger.Info("Initializing: Only localhost access available. No Internet.")
	}

	/************************************************************************************/
	/********  WiFi Configured, proceed with processing *********************************/

	// Start the background processing threads now that we are connected to WiFi
	logger.Info(">>>>> Starting remaining background processes")

	go processSensorData(ctx, &wg, logger)     // Background thread to process sensor data
	go removeExpiredSensors(ctx, &wg, logger)  // Background thread to process expired sensors
	go monitorWiFi(ctx, &wg, lightCMD, logger) // Background thread to monitor Wi-Fi connection and handle setup if needed

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
			stopLight()
			wg.Wait()
			logger.Info("!!!!! All background processing halted. Exiting SensorHub.")
		case syscall.SIGINT:
			logger.Debug("SIGINT received by main thread; waiting for goroutines to exit")
			stopLight()
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
