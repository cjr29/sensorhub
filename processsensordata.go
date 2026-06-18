package main

import (
	"time"
)

func processSensorData() {
	for {
		amt := 10 // seconds delay
		time.Sleep(time.Duration(amt) * time.Second)
		logger.Info("Processing sensor data every", "seconds", amt)
	}
}
