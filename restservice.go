package main

import (
	"context"
	"time"
)

func restService(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err() // quit
		default:
			amt := 120 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info("REST service still running ...")
		}
	}
}
