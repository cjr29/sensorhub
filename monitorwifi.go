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
			logger.Info("Shutting down Wi-Fi monitoring thread.")
			return // quit
		default:
			amt := 3600 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("Check WiFi availability every %d seconds", amt))
		}
	}
}
