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
			logger.Debug("!!!!! Shutting down Wi-Fi monitoring thread.")
			return // quit
		default:
			amt := 10 // seconds delay between checks
			logger.Debug(fmt.Sprintf("Check WiFi availability every %d seconds", amt))
			// See if Internet (remote) access is available
			if hasInternet() {
				logger.Debug("Internet access confirmed")
			} else {
				logger.Debug("Only localhost access available. No Internet.")
			}
			time.Sleep(time.Duration(amt) * time.Second)
		}
	}
}
