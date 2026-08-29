package main

import (
	"context"
	"fmt"
	"time"

	"periph.io/x/conn/v3/gpio"
)

const SLOW = 1.0 * time.Second
const FAST = 4.0 * time.Second

const (
	OFF int = iota
	REDON
	GREENON
	REDSLOW
	GREENSLOW
	REDFAST
	GREENFAST
)

const RED = "GPIO23"
const GREEN = "GPIO17"

var redLight *LEDGPIO
var greenLight *LEDGPIO

// Primary function to cause an LED to blink or remain steady on or off.
// Since the LED has two colors, red and green, the LED can be set to red, green, or off.
// The LED can also be set to blink at a specified rate in Hz (blinks per second).
// This calling function ensures that the LED is only powered by one GPIO pin at a time. The LED is powered by GPIO4 for red and GPIO17 for green.
// LED can be in only one state at a time as commanded by the input channel.
// Input is a constant defined above. The LED can be set to REDON, GREENON, OFF, REDSLOW, GREENSLOW, REDFAST, or GREENFAST.
// Funtion returns when the context is canceled, which occurs when the program receives a SIGTERM signal or
// cancel() via stopLED() in main.go. The LED is turned off before the function returns.

// blinkLED runs in a separate goroutine
func blinkLight(led *LEDGPIO, ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Debug(fmt.Sprintf("Started blinking for %s.", led.Name))
	for {
		select {
		case <-ctx.Done():
			led.Off() // Ensure LED is off when stopping
			logger.Debug(fmt.Sprintf("Blinking stopped for %s.", led.Name))
			ticker.Stop()
			greenLight.Off()
			redLight.Off()
			return // Exit goroutine
		case <-ticker.C:
			led.Toggle()
			// logger.Debug("Blink LED")
		}
	}
}

// Runs a a background goroutine to control the activity LED based on commands received from the input channel.
func lightControl(ctx context.Context, cmd chan int) {

	logger.Info("Starting Activity/WiFi light background thread.")

	var rate time.Duration
	var currentState int = OFF

	ctxBlink, stopBlink := context.WithCancel(context.Background())

	for {
		select {
		case <-ctx.Done():
			// Turn off the LED before exiting.
			stopBlink()
			greenLight.Off()
			redLight.Off()
			releasePin(redLight.Pin)
			releasePin(greenLight.Pin)
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
					redLight.Off()
					greenLight.Off()
				case REDON:
					if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
						stopBlink()
					}
					rate = 0
					greenLight.Off()
					redLight.On()
				case GREENON:
					if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
						stopBlink()
					}
					rate = 0
					redLight.Off()
					greenLight.On()
				case REDSLOW:
					stopBlink()
					greenLight.Off()
					redLight.Off()
					rate = SLOW
					go blinkLight(redLight, ctxBlink, rate)
				case GREENSLOW:
					stopBlink()
					greenLight.Off()
					redLight.Off()
					rate = SLOW
					go blinkLight(greenLight, ctxBlink, rate)
				case REDFAST:
					stopBlink()
					greenLight.Off()
					redLight.Off()
					rate = FAST
					go blinkLight(redLight, ctxBlink, rate)
				case GREENFAST:
					stopBlink()
					greenLight.Off()
					redLight.Off()
					rate = FAST
					go blinkLight(greenLight, ctxBlink, rate)
				default:
					logger.Error("Invalid LED state", "state", currentState)
				}
			}
		}
	}
}

func releasePin(pin gpio.PinOut) {
	if err := pin.Halt(); err != nil {
		logger.Debug(fmt.Sprintf("failed to release GPIO: %v", err))
	}
	return
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
