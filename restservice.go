package main

import (
	"context"
	"fmt"
	"time"
)

func restService(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down REST service thread.")
			return ctx.Err() // quit
		default:
			amt := 3600 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("REST service still running (checked every %d seconds) ...", amt))
		}
	}
}
