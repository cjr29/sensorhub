package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	configureLogging()

	// Check if sensor hub is already configured to connect to a Wi-Fi network. If not, start the hotspot and web server for setup.
	logger.Debug("Check if Sensor Hub config file exists")
	if isConfigFile() {
		logger.Debug("Config file exists, check if it has an SSID and PWD")
		if err := currentWiFi.getSSIDPWD(); err == nil {
			logger.Info(fmt.Sprintf("Already configured to connect to Wi-Fi SSID:%s. Now test connection.", currentWiFi.SSID))
			if hasInternet() {
				logger.Info(fmt.Sprintf("WiFi verified on and connected to %s.", currentWiFi.SSID))
			} else {
				logger.Info(fmt.Sprintf("Configured Wi-Fi network %s not currently reachable. Starting hotspot configuration.", currentWiFi.SSID))
				logger.Debug("Calling loginWifiViaHotspot()")
				// Start local Web server on :8080 to take commands from client
				// index, list available SSIDs, logon to SSID

				/*******************/
				loginWifiViaHotspot()
				/*******************/

				logger.Info(fmt.Sprintf("WiFi connected to %s", currentWiFi.SSID))
			}
		} else {
			logger.Info("WiFi has not been configured. Starting hotspot configuration.")
			logger.Debug("Calling loginViaHotspot()")
			//configureViaHotspot()
			loginWifiViaHotspot()
			logger.Info(fmt.Sprintf("WiFi connected to %s", currentWiFi.SSID))
		}
	} else {
		// SSID not configured
		logger.Debug("Sensor Hub config file does not exist, call loginViaHotSpot()")
		loginWifiViaHotspot()
		logger.Debug(fmt.Sprintf("Returned from loginViaHotspot. WiFi connected to %s", currentWiFi.SSID))
	}
	logger.Debug("WiFi and Internet access verified")

	/************************************************************************************/
	/********  WiFi Configured, proceed with processing *********************************/

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger.Debug("Retrieved program context to handle signals")

	// signals: SIGINT/SIGTERM -> shutdown, SIGHUP -> reopen log
	logger.Debug("Set up channel to catch shutdown signals")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Watches for system signals in a separate goroutine so it doesn't block the main work loop
	logger.Debug("Start go function to monitor signals.")
	go func() {
		for {
			sig := <-sigCh
			switch sig {
			case syscall.SIGHUP:
				// reopen log file after logrotate signal received
				if err := logFh.Reopen(logFile); err != nil {
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
	logger.Debug("Activated go function to watch for system signals")

	logger.Info("Starting background processes")

	// Start the background processing threads now that we are connected to WiFi
	go processSensorData(ctx)    // Background thread to process sensor data
	go restService(ctx)          // Background thread to log that the REST service is still running
	go removeExpiredSensors(ctx) // Background thread to process expired sensors
	go monitorWiFi(ctx)          // Background thread to monitor Wi-Fi connection and handle setup if needed

	logger.Info("Sensor Hub setup complete.")

	// Wait for cancellation and exit
	<-ctx.Done()
	logger.Info("Sensor Hub stopped")
	os.Exit(0)
}
