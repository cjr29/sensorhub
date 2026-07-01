package main

import (
	"context"
	"fmt"
)

func monitorWiFi(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Debug("!!!!! Shutting down Wi-Fi monitoring thread.")
			return // quit
		default:
			amt := 360 // seconds delay between checks
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
