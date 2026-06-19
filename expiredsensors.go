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
			return ctx.Err() // quit
		default:
			amt := 300 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("Remove expired sensors every %d seconds", amt))
		}
	}
}
