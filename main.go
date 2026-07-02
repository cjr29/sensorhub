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
	// Create a context that will be canceled when SIGTERM signal is received
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	defer stop()
	var wg sync.WaitGroup

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

	// Start the background processing threads now that we are connected to WiFi
	logger.Info("Starting background processes")
	wg.Add(4)
	go func() {
		defer wg.Done()
		processSensorData(ctx, &wg)    // Background thread to process sensor data
		restService(ctx, &wg)          // Background thread to log that the REST service is still running
		removeExpiredSensors(ctx, &wg) // Background thread to process expired sensors
		monitorWiFi(ctx, &wg)          // Background thread to monitor Wi-Fi connection and handle setup if needed
	}()

	// Handle other signals while waiting for goroutines to halt.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig,
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGHUP,
		// add more signals you care about
	)
	defer signal.Stop(sig)

	// Main program loop watching for system control signals and either reopening logs or terminating processing
	for {
		select {
		case s := <-sig:
			// Handle "other" signals without terminating immediately.
			logger.Debug(fmt.Sprintf("received signal: %v", sig))
			if s == syscall.SIGTERM {
				// SIGTERM is handled by the context cancellation above, so just log and continue.
				logger.Debug("SIGTERM received by main thread; waiting for goroutines to exit")
				// Received SIGTERM, Wait for workers to halt.
				wg.Wait()
				logger.Debug("all goroutines halted; exiting")
			}
			if s == syscall.SIGINT {
				// If you want Ctrl+C to also initiate shutdown:
				// stop() will cancel ctx, but only if ctx is based on SIGTERM.
				// You can instead create a ctx that cancels on both SIGTERM and SIGINT.
				//
				// For now, just log and continue.
			}
			if s == syscall.SIGHUP {
				// watch for log rotations and reopen log
				logger.Debug("SIGHUP received by main thread.")
				// reopen log file after logrotate signal received
				if err := logFh.Reopen(logFile); err != nil {
					logger.Error("Failed to reopen log file", "err", err)
				} else {
					logger.Debug("Reopened log file after SIGHUP")
				}
				// Stay in signal loop, don't return to main thread
			}
		case <-time.After(250 * time.Millisecond):
			// keep loop responsive; or omit if you’re fine blocking on channels
		}
	}
}

// for {
// 	sig := <-sigCh
// 	switch sig {
// 	case syscall.SIGHUP:
// 		// watch for log rotations and reopen log
// 		logger.Debug("SIGHUP received by main thread.")
// 		// reopen log file after logrotate signal received
// 		if err := logFh.Reopen(logFile); err != nil {
// 			logger.Error("Failed to reopen log file", "err", err)
// 		} else {
// 			logger.Debug("Reopened log file after SIGHUP")
// 		}
// 		// Stay in signal loop, don't return to main thread
// 	case syscall.SIGINT:
// 		logger.Debug("----- SIGINT received by main thread. Continue background processing.")

// 	case syscall.SIGTERM:
// 		logger.Debug("!!!!! SIGTERM received by main thread. Stopping background processing.")
// 		// Give workers time to exit
// 		cancel()
// 		done := make(chan struct{})
// 		go func() {
// 			wg.Wait()
// 			close(done)
// 		}()

// 		select {
// 		case <-done:
// 			logger.Info("!!!!! All background processing has stopped.")
// 			logger.Info("!!!!! Sensor Hub stopped")
// 			return
// 		case <-time.After(10 * time.Second):
// 			logger.Info("!!!!! Timeout waiting for background processing to stop.")
// 			logger.Info("!!!!! Sensor Hub stopped")
// 			return
// 		}
// 	}
// }
// }
