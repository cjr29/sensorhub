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

	// Set up signals and wait group for goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

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
	logger.Info("Starting background processes")
	// Start the background processing threads now that we are connected to WiFi
	go processSensorData(ctx, &wg)    // Background thread to process sensor data
	go restService(ctx, &wg)          // Background thread to log that the REST service is still running
	go removeExpiredSensors(ctx, &wg) // Background thread to process expired sensors
	go monitorWiFi(ctx, &wg)          // Background thread to monitor Wi-Fi connection and handle setup if needed

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
					logger.Debug("Reopened log file after SIGHUP")
				}
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Debug(fmt.Sprintf("Shutting down: %s signal received ...", sig.String()))
				// Signal everyone to halt
				cancel()
				// Wait until they have all halted
				wg.Wait()
				logger.Info("All background processing stopped.")
				return
			}
		}
	}()
	logger.Info("Sensor Hub setup complete.")

	// Wait for cancellation and exit
	logger.Debug("main is waiting for ctx.Done()")
	<-ctx.Done()
	logger.Info("Sensor Hub stopped")
	time.Sleep(time.Duration(2) * time.Second)
	os.Exit(0)
}
