package main

import (
	"context"
	"fmt"
	"os"
)

const ledStateFile = "/var/sensorhub/led.state"

var stateNames = map[string]int{
	"OFF":       OFF,
	"REDON":     REDON,
	"GREENON":   GREENON,
	"REDSLOW":   REDSLOW,
	"GREENSLOW": GREENSLOW,
	"REDFAST":   REDFAST,
	"GREENFAST": GREENFAST,
}

// writeLEDState converts the integer state to its file representation.
func writeLEDState(state int) error {
	var stateText string

	switch state {
	case OFF:
		stateText = "OFF"
	case REDON:
		stateText = "REDON"
	case GREENON:
		stateText = "GREENON"
	case REDSLOW:
		stateText = "REDSLOW"
	case GREENSLOW:
		stateText = "GREENSLOW"
	case REDFAST:
		stateText = "REDFAST"
	case GREENFAST:
		stateText = "GREENFAST"
	default:
		return fmt.Errorf("invalid LED state: %d", state)
	}

	return os.WriteFile(
		ledStateFile,
		[]byte(stateText+"\n"),
		0644,
	)
}

// RunLEDStateWriter processes state commands serially from the channel.
func RunLEDStateWriter(ctx context.Context, states <-chan int) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case state, ok := <-states:
			if !ok {
				return nil
			}

			if err := writeLEDState(state); err != nil {
				return err
			}
		}
	}
}
