package main

import (
	"context"
	"fmt"
	"time"
)

func processSensorData(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err() // quit
		default:
			amt := 10 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("Processing sensor data every %d seconds", amt))
		}
	}
}
