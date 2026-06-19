package main

import (
	"time"
)

func removeExpiredSensors() {
	for {
		amt := 120 // seconds delay
		time.Sleep(time.Duration(amt) * time.Second)
		logger.Info("Remove expired sensors every", "seconds", amt)
	}
}
