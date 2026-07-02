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

func main() {

	configureLogging()

	// Check if sensor hub is already connected to a Wi-Fi network. If not, start the hotspot and web server for setup.
	// Existence of the conf file is evidence that configuration has occurred. Then, need to test connection.
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
		logger.Info("Internet access confirmed")
	} else {
		logger.Info("Only localhost access available. No Internet.")
	}

	/************************************************************************************/
	/********  WiFi Configured, proceed with processing *********************************/

	// signals: SIGINT/SIGTERM -> shutdown, SIGHUP -> reopen log
	logger.Debug("Set up channel to catch shutdown signals")
	// Context canceled on SIGTERM (systemd stop)
	const workers = 4 // 4 background processes
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(4)
	logger.Info("Starting background processes")
	// Start the background processing threads now that we are connected to WiFi

	go func() {
		defer wg.Done()
		processSensorData(ctx) // Background thread to process sensor data
	}()
	go func() {
		defer wg.Done()
		restService(ctx) // Background thread to log that the REST service is still running
	}()
	go func() {
		defer wg.Done()
		removeExpiredSensors(ctx) // Background thread to process expired sensors
	}()
	go func() {
		defer wg.Done()
		monitorWiFi(ctx) // Background thread to monitor Wi-Fi connection and handle setup if needed
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	// Main program loop watching for system control signals and either reopening logs or terminating processing
	for {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP:
			// watch for log rotations and reopen log
			logger.Debug("SIGHUP received by main thread.")
			// reopen log file after logrotate signal received
			if err := logFh.Reopen(logFile); err != nil {
				logger.Error("Failed to reopen log file", "err", err)
			} else {
				logger.Debug("Reopened log file after SIGHUP")
			}
			// Stay in signal loop, don't return to main thread
		case syscall.SIGINT:
			logger.Debug("----- SIGINT received by main thread. Continue background processing.")

		case syscall.SIGTERM:
			logger.Debug("!!!!! SIGTERM received by main thread. Stopping background processing.")
			// Give workers time to exit
			cancel()
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				logger.Info("!!!!! All background processing has stopped.")
				logger.Info("!!!!! Sensor Hub stopped")
				return
			case <-time.After(10 * time.Second):
				logger.Info("!!!!! Timeout waiting for background processing to stop.")
				logger.Info("!!!!! Sensor Hub stopped")
				return
			}
		}
	}
}
