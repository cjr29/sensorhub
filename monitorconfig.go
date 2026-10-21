package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"
)

const hubConfigMonitorInterval = 5 * time.Second

func monitorHubConfig(ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger, initiallyConfigured bool) {
	defer wg.Done()

	ticker := time.NewTicker(hubConfigMonitorInterval)
	defer ticker.Stop()

	configPresent := initiallyConfigured
	hotspotStarted := !initiallyConfigured

	for {
		select {
		case <-ctx.Done():
			logger.Info("!!!!! Shutting down hub configuration monitoring thread.")
			return
		case <-ticker.C:
			_, err := os.Stat(pathToHubConfig)
			if err == nil {
				configPresent = true
				hotspotStarted = false
				continue
			}
			if !errors.Is(err, os.ErrNotExist) {
				logger.Error("Unable to check Wi-Fi configuration file", "err", err)
				continue
			}

			if !configPresent && hotspotStarted {
				continue
			}

			logger.Info("Wi-Fi configuration file is missing. Starting hotspot.")
			if err := startHotspot(); err != nil {
				logger.Error("Failed to start hotspot after Wi-Fi configuration file disappeared", "err", err)
				configPresent = false
				continue
			}

			configPresent = false
			hotspotStarted = true
		}
	}
}
