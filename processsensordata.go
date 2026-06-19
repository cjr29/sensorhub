package main

import (
	"fmt"
	"time"
)

func processSensorData() {
	for {
		amt := 10 // seconds delay
		time.Sleep(time.Duration(amt) * time.Second)
		logger.Info(fmt.Sprintf("Processing sensor data every %d seconds", amt))
	}
}
