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
			logger.Debug("!!!!! Shutting down remove expired sensors thread.")
			time.Sleep(time.Duration(1) * time.Second)
			return // quit
		default:
			amt := 3600 * 24 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Debug(fmt.Sprintf("Remove expired sensors every %d seconds", amt))
		}
	}
}
