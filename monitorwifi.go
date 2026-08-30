package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

func monitorWiFi(ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			logger.Info("!!!!! Shutting down Wi-Fi monitoring thread.")
			return // quit
		default:
			amt := 3600 // seconds delay between checks
			logger.Debug(fmt.Sprintf("Check WiFi availability every %d seconds", amt))
			// See if Internet (remote) access is available
			if hasInternet() {
				leds.SetState(GREENON) // Set LED to green on to indicate WiFi is configured and connected
				logger.Debug("Internet access confirmed")
			} else {
				leds.SetState(GREENSLOW) // Set LED to green slow to indicate WiFi is configured but no Internet access
				logger.Debug("Only localhost access available. No Internet.")
			}
			time.Sleep(time.Duration(amt) * time.Second)
		}
	}
}
