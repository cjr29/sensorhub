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
			return // quit
		default:
			amt := 1 * 24 // seconds delay
			logger.Debug(fmt.Sprintf("Remove expired sensors every %d seconds", amt))
			time.Sleep(time.Duration(amt) * time.Second)
		}
	}
}
