package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func removeExpiredSensors(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down expired sensors processing thread.")
			time.Sleep(time.Duration(1) * time.Second)
			return // quit
		default:
			amt := 5 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("Remove expired sensors every %d seconds", amt))
		}
	}
}
