package main

import (
	"context"
	"fmt"
	"time"
)

func removeExpiredSensors(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Debug("!!!!! Shutting down remove expired sensors thread.")
			return // quit
		default:
			amt := 3600 * 24 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Debug(fmt.Sprintf("Remove expired sensors every %d seconds", amt))
		}
	}
}
