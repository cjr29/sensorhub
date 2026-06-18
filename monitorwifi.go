package main

import (
	"time"
)

func monitorWiFi() {
	for {
		amt := 60 // seconds delay
		time.Sleep(time.Duration(amt) * time.Second)
		logger.Info("Check WiFi availability every", "seconds", amt)
	}
}
