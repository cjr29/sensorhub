package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const AppVersion = "0.1.1"

var hostName string

func main() {
	// Create a context that will be canceled when SIGTERM signal is received
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	defer stop()
	var wg sync.WaitGroup

	configureLogging()

	// Retrieve hostname and log it
	var err error
	hostName, err = getHostname()
	if err != nil {
		logger.Error("Failed to retrieve hostname", "err", err)
	} else {
		logger.Info(fmt.Sprintf("Sensor Hub Hostname: %s", hostName))
	}

	// Check if sensor hub is already connected to a Wi-Fi network. If not, start the hotspot and web server for setup.
	// Existence of the conf file is evidence that configuration has occurred. Then, need to test connection.
	logger.Info("App Version: " + AppVersion)
	logger.Info("Check if Sensor Hub WiFi has been configured.")
	if !wifiConfigured() {
		// WiFi not configured. Use hotspot configuration.
		loginWiFiViaHotspot()
	} else {
		logger.Info(fmt.Sprintf("WiFi has been preconfigured. Using preset SSID %s", currentWiFi.SSID))
	}

	logger.Info(fmt.Sprintf("WiFi connected to %s", currentWiFi.SSID))
	logger.Debug("Pause 2 seconds for WiFi to stabilize before checking Internet access.")
	time.Sleep(time.Duration(2) * time.Second)

	// See if Internet (remote) access is available
	if hasInternet() {
		logger.Info("Initializing: Internet access confirmed")
	} else {
		logger.Info("Initializing: Only localhost access available. No Internet.")
	}

	/************************************************************************************/
	/********  WiFi Configured, proceed with processing *********************************/

	// Start the background processing threads now that we are connected to WiFi
	logger.Info(">>>>> Starting background processes")
	wg.Add(4)
	defer wg.Done()
	go processSensorData(ctx, &wg, logger)    // Background thread to process sensor data
	go restService(ctx, &wg, logger)          // Background thread to log that the REST service is still running
	go removeExpiredSensors(ctx, &wg, logger) // Background thread to process expired sensors
	go monitorWiFi(ctx, &wg, logger)          // Background thread to monitor Wi-Fi connection and handle setup if needed

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
			wg.Wait()
			logger.Info("!!!!! All background processing halted. Exiting SensorHub.")
		case syscall.SIGINT:
			logger.Debug("SIGINT received by main thread; waiting for goroutines to exit")
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
