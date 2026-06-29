package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func processSensorData(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			logger.Info("!!!!! Shutting down sensor data processing thread.")
			time.Sleep(time.Duration(1) * time.Second)
			return // quit
		default:
			amt := 6 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("Processing sensor data every %d seconds", amt))
		}
	}
}
