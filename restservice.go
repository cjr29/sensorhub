package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func restService(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			logger.Info("!!!!! Shutting down REST service thread.")
			time.Sleep(time.Duration(1) * time.Second)
			return // quit
		default:
			amt := 3600 // seconds delay
			time.Sleep(time.Duration(amt) * time.Second)
			logger.Info(fmt.Sprintf("REST service still running (checked every %d seconds) ...", amt))
		}
	}
}
