package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func monitorWiFi(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			logger.Info("!!!!! Shutting down Wi-Fi monitoring thread.")
			return // quit
		default:
			amt := 360 // seconds delay between checks
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Debug(fmt.Sprintf("Check WiFi availability every %d seconds", amt))

			// See if Internet (remote) access is available
			if hasInternet() {
				logger.Info("Internet access confirmed")
			} else {
				logger.Info("Only localhost access available. No Internet.")
				// Start hotspot to configure wifi
				// loginWiFiViaHotspot()
			}

		}
	}
}
