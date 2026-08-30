package main

import (
	"context"
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
			amt := 60 // seconds delay between checks
			// logger.Debug(fmt.Sprintf("Check WiFi availability every %d seconds", amt))
			// See if Internet (remote) access is available
			if hasInternet() {
				internet = true
				// logger.Debug("Internet access confirmed")
			} else {
				internet = false
				// logger.Debug("Only localhost access available. No Internet.")
			}
			time.Sleep(time.Duration(amt) * time.Second)
		}
	}
}
