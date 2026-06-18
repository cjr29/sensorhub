package main

import (
	"time"
)

func restService() {
	for {
		amt := 30 // seconds delay
		time.Sleep(time.Duration(amt) * time.Second)
		logger.Info("REST service still running ...")
	}
}
