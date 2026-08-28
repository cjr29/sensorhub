package main

import (
	"context"
	"fmt"
	"time"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
)

const RED = "GPIO23"
const GREEN = "GPIO17"
const SLOW = 1.0
const FAST = 4.0

const (
	OFF int = iota
	REDON
	GREENON
	REDSLOW
	GREENSLOW
	REDFAST
	GREENFAST
)

// Primary function to cause an LED to blink or remain steady on or off.
// Since the LED has two colors, red and green, the LED can be set to red, green, or off.
// The LED can also be set to blink at a specified rate in Hz (blinks per second).
// This calling function ensures that the LED is only powered by one GPIO pin at a time. The LED is powered by GPIO4 for red and GPIO17 for green.
// LED can be in only one state at a time as commanded by the input channel.
// Input is a constant defined above. The LED can be set to REDON, GREENON, OFF, REDSLOW, GREENSLOW, REDFAST, or GREENFAST.
// Funtion returns when the context is canceled, which occurs when the program receives a SIGTERM signal or
// cancel() via stopLED() in main.go. The LED is turned off before the function returns.

func blinkActivityLED(ctx context.Context, cmd chan int) {

	logger.Info("Starting Activity LED background thread.")

	// Initialize the GPIO pins for the red and green LEDs.
	redPin, err := pinFromName(RED)
	if err != nil {
		logger.Error("Failed to initialize red LED pin", "err", err)
		return
	}
	greenPin, err := pinFromName(GREEN)
	if err != nil {
		logger.Error("Failed to initialize green LED pin", "err", err)
		return
	}

	var rate float64
	var currentState int = OFF

	ctxBlink, stopBlink := context.WithCancel(context.Background())

	for {
		select {
		case <-ctx.Done():
			_ = redPin.Out(gpio.Low)
			_ = greenPin.Out(gpio.Low)
			// Turn off the LED before exiting.
			stopBlink()
			return

		case newState := <-cmd:
			if newState != currentState {
				currentState = newState
				switch currentState {
				case OFF:
					if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
						stopBlink()
					}
					rate = 0
					_ = redPin.Out(gpio.Low)
					_ = greenPin.Out(gpio.Low)
				case REDON:
					if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
						stopBlink()
					}
					rate = 0
					_ = redPin.Out(gpio.High)
					_ = greenPin.Out(gpio.Low)
				case GREENON:
					if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
						stopBlink()
					}
					rate = 0
					_ = redPin.Out(gpio.Low)
					_ = greenPin.Out(gpio.High)
				case REDSLOW:
					stopBlink()
					rate = SLOW
					if err := blink(ctxBlink, RED, rate); err != nil {
						logger.Error("Failed to blink red LED", "err", err)
					}
				case GREENSLOW:
					stopBlink()
					rate = SLOW
					if err := blink(ctxBlink, GREEN, rate); err != nil {
						logger.Error("Failed to blink green LED", "err", err)
					}
				case REDFAST:
					stopBlink()
					rate = FAST
					if err := blink(ctxBlink, RED, rate); err != nil {
						logger.Error("Failed to blink red LED", "err", err)
					}
				case GREENFAST:
					stopBlink()
					rate = FAST
					if err := blink(ctxBlink, GREEN, rate); err != nil {
						logger.Error("Failed to blink green LED", "err", err)
					}
				default:
					logger.Error("Invalid LED state", "state", currentState)
				}
			}
		}

		// if rate > 0 {
		// 	if err := blink(ctx, RED, rate); err != nil {
		// 		logger.Error("Failed to blink red LED", "err", err)
		// 	}
		// 	if err := blink(ctx, GREEN, rate); err != nil {
		// 		logger.Error("Failed to blink green LED", "err", err)
		// 	}
		// }
	}
}

/*************************************************************************/
/**                          Helper functions                           **/
/*************************************************************************/

// blink starts blinking pin at rateHz complete blink cycles per second.
// For example, rateHz == 2 means two on/off cycles per second.
//
// The goroutine stops when ctx is canceled. The LED is switched off before
// the function returns.
func blink(ctx context.Context, pinName string, rateHz float64) error {

	// Convert pin name to gpio.PinIO
	pin := gpioreg.ByName(pinName)
	if pin == nil {
		return fmt.Errorf("Invalid pin: %s", pinName)
	}

	if rateHz <= 0 {
		return fmt.Errorf("blink rate must be greater than zero")
	}

	// One complete cycle consists of an ON interval and an OFF interval.
	halfPeriod := time.Duration(float64(time.Second) / (2 * rateHz))
	if halfPeriod <= 0 {
		return fmt.Errorf("blink rate is too high")
	}

	if err := pin.Out(gpio.Low); err != nil {
		return fmt.Errorf("initializing GPIO: %w", err)
	}

	defer func() {
		// Leave the LED switched off when stopping.
		_ = pin.Out(gpio.Low)
	}()

	ticker := time.NewTicker(halfPeriod)
	defer ticker.Stop()

	level := gpio.Low

	for {
		select {
		case <-ctx.Done():
			pin.Out(gpio.Low) // Ensure LED is off when stopping
			return nil

		case <-ticker.C:
			if level == gpio.Low {
				level = gpio.High
			} else {
				level = gpio.Low
			}

			if err := pin.Out(level); err != nil {
				return fmt.Errorf("writing GPIO: %w", err)
			}
		}
	}
}

func pinFromName(name string) (gpio.PinIO, error) {
	pin := gpioreg.ByName(name)
	if pin == nil {
		return nil, fmt.Errorf("pin %s not found", name)
	}

	pinIO, ok := pin.(gpio.PinIO)
	if !ok {
		return nil, fmt.Errorf("pin %s does not support input/output", name)
	}

	return pinIO, nil
}

/***********************************************
// PinIO definitions

type PinIO interface {
 pin.Pin
 // PinIn
 In(pull Pull, edge Edge) error
 Read() Level
 WaitForEdge(timeout time.Duration) bool
 Pull() Pull
 DefaultPull() Pull
 // PinOut
 Out(l Level) error
 PWM(duty Duty, f physic.Frequency) error
}

PinIO is a GPIO pin that supports both input and output. It matches both interfaces PinIn and PinOut.

A GPIO pin implementing PinIO may fail at either input or output or both.

func (pin.Pin) Function() string
func (conn.Resource) Halt() error
func (pin.Pin) Name() string
func (pin.Pin) Number() int
func (conn.Resource) String() string

gpio.PinIO on pkg.go.dev

******************************************/
