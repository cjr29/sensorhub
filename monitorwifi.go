package main

import (
	"context"
	"fmt"
	"time"
)

func monitorWiFi(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Debug("!!!!! Shutting down Wi-Fi monitoring thread.")
			return // quit
		default:
			amt := 360 // seconds delay between checks
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Debug(fmt.Sprintf("Check WiFi availability every %d seconds", amt))

			// See if Internet (remote) access is available
			if hasInternet() {
				logger.Debug("Internet access confirmed")
			} else {
				logger.Debug("Only localhost access available. No Internet.")
				// Start hotspot to configure wifi
				// loginWiFiViaHotspot()
			}

		}
	}
}
