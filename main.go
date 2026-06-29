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

	const workers = 4 // 4 background processes

	configureLogging()

	// Set up signals and wait group for goroutines
	// ctx, cancel := context.WithCancel(context.Background())

	// Context canceled on SIGTERM (systemd stop)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)

	defer stop()
	// defer cancel()
	var wg sync.WaitGroup
	wg.Add(workers)

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

	// Watch for system signals in a separate goroutine so it doesn't block the main work loop
	logger.Debug("Start go function to monitor signals.")
	// Go function to watch for log rotations and reopen log
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
				return
			}
		}
	}()

	logger.Info("Sensor Hub setup complete.")

	// Wait until background threads have halted
	<-ctx.Done()
	wg.Wait()
	// Wait for cancellation and exit
	// Signal everyone to halt
	logger.Info("All background processing has stopped.")
	logger.Info("Sensor Hub stopped")
	os.Exit(0)
}
