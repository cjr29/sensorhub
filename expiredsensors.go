package main

import (
	"context"
	"fmt"
	"time"
)

func removeExpiredSensors(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down expired sensors processing thread.")
			return ctx.Err() // quit
		default:
			amt := 10 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("Remove expired sensors every %d seconds", amt))
		}
	}
}
