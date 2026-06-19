package main

import (
	"fmt"
	"time"
)

func monitorWiFi() {
	for {
		amt := 60 // seconds delay
		time.Sleep(time.Duration(amt) * time.Second)
		logger.Info(fmt.Sprintf("Check WiFi availability every %d seconds", amt))
	}
}
