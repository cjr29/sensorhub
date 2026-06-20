package main

import (
	"context"
	"fmt"
	"time"
)

func monitorWiFi(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down Wi-Fi monitoring thread.")
			return ctx.Err() // quit
		default:
			amt := 20 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("Check WiFi availability every %d seconds", amt))
		}
	}
}
