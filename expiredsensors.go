package main

import (
	"fmt"
	"time"
)

func removeExpiredSensors() {
	for {
		amt := 120 // seconds delay
		time.Sleep(time.Duration(amt) * time.Second)
		logger.Info(fmt.Sprintf("Remove expired sensors every %d seconds", amt))
	}
}
