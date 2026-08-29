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
	logger.Debug("LED started blinking...")
	for {
		select {
		case <-ctx.Done():
			led.On() // Ensure LED is on when stopping
			// logger.Debug("Blinking stopped.")
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
				case REDSLOW:
					stopBlink()
					rate = SLOW
					go blinkLight(redLight, ctxBlink, rate)
				case GREENSLOW:
					stopBlink()
					rate = SLOW
					go blinkLight(greenLight, ctxBlink, rate)
				case REDFAST:
					stopBlink()
					rate = FAST
					go blinkLight(redLight, ctxBlink, rate)
				case GREENFAST:
					stopBlink()
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

// func blinkLight(ctx context.Context, cmd chan int) {

// 	logger.Info("Starting Activity LED background thread.")

// 	// Initialize the GPIO pins for the red and green LEDs.
// 	redPin, err := pinFromName(RED)
// 	if err != nil {
// 		logger.Error("Failed to initialize red LED pin", "err", err)
// 		return
// 	}
// 	greenPin, err := pinFromName(GREEN)
// 	if err != nil {
// 		logger.Error("Failed to initialize green LED pin", "err", err)
// 		return
// 	}

// 	var rate float64
// 	var currentState int = OFF

// 	ctxBlink, stopBlink := context.WithCancel(context.Background())

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			_ = redPin.Out(gpio.Low)
// 			_ = greenPin.Out(gpio.Low)
// 			// Turn off the LED before exiting.
// 			stopBlink()
// 			return

// 		case newState := <-cmd:
// 			if newState != currentState {
// 				currentState = newState
// 				switch currentState {
// 				case OFF:
// 					if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
// 						stopBlink()
// 					}
// 					rate = 0
// 					_ = redPin.Out(gpio.Low)
// 					_ = greenPin.Out(gpio.Low)
// 				case REDON:
// 					if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
// 						stopBlink()
// 					}
// 					if err := redPin.Halt(); err != nil {
// 						logger.Debug(fmt.Sprintf("failed to release GPIO: %v", err))
// 					}
// 					rate = 0
// 					_ = redPin.Out(gpio.High)
// 					_ = greenPin.Out(gpio.Low)
// 				case GREENON:
// 					if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
// 						stopBlink()
// 					}
// 					if err := greenPin.Halt(); err != nil {
// 						logger.Debug(fmt.Sprintf("failed to release GPIO: %v", err))
// 					}
// 					rate = 0
// 					_ = redPin.Out(gpio.Low)
// 					_ = greenPin.Out(gpio.High)
// 				case REDSLOW:
// 					stopBlink()
// 					if err := redPin.Halt(); err != nil {
// 						logger.Debug(fmt.Sprintf("failed to release GPIO: %v", err))
// 					}
// 					rate = SLOW
// 					go blink(ctxBlink, RED, rate)
// 				case GREENSLOW:
// 					stopBlink()
// 					if err := greenPin.Halt(); err != nil {
// 						logger.Debug(fmt.Sprintf("failed to release GPIO: %v", err))
// 					}
// 					rate = SLOW
// 					go blink(ctxBlink, GREEN, rate)
// 				case REDFAST:
// 					stopBlink()
// 					if err := redPin.Halt(); err != nil {
// 						logger.Debug(fmt.Sprintf("failed to release GPIO: %v", err))
// 					}
// 					rate = FAST
// 					go blink(ctxBlink, RED, rate)
// 				case GREENFAST:
// 					stopBlink()
// 					if err := greenPin.Halt(); err != nil {
// 						logger.Debug(fmt.Sprintf("failed to release GPIO: %v", err))
// 					}
// 					rate = FAST
// 					go blink(ctxBlink, GREEN, rate)
// 				default:
// 					logger.Error("Invalid LED state", "state", currentState)
// 				}
// 			}
// 		}
// 	}
// }

/*************************************************************************/
/**                          Helper functions                           **/
/*************************************************************************/

// blink starts blinking pin at rateHz complete blink cycles per second.
// For example, rateHz == 2 means two on/off cycles per second.
//
// The goroutine stops when ctx is canceled. The LED is switched off before
// the function returns.
// func blink(ctx context.Context, pinName string, rateHz float64) {

// 	// Convert pin name to gpio.PinIO
// 	pin := gpioreg.ByName(pinName)
// 	if pin == nil {
// 		return
// 	}

// 	if rateHz <= 0 {
// 		return
// 	}

// 	// One complete cycle consists of an ON interval and an OFF interval.
// 	halfPeriod := time.Duration(float64(time.Second) / (2 * rateHz))
// 	if halfPeriod <= 0 {
// 		return
// 	}

// 	if err := pin.Out(gpio.Low); err != nil {
// 		return
// 	}

// 	defer func() {
// 		// Leave the LED switched off when stopping.
// 		_ = pin.Out(gpio.Low)
// 	}()

// 	ticker := time.NewTicker(halfPeriod)
// 	defer ticker.Stop()

// 	level := gpio.Low

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			pin.Out(gpio.Low) // Ensure LED is off when stopping
// 			return

// 		case <-ticker.C:
// 			if level == gpio.Low {
// 				level = gpio.High
// 			} else {
// 				level = gpio.Low
// 			}

// 			if err := pin.Out(level); err != nil {
// 				return
// 			}
// 		}
// 	}
// }

// func stopBlink(pinName string) {
// 	pin := gpioreg.ByName(pinName)
// 	pin.In(gpio.PullNoChange, gpio.NoEdge)
// }

// func pinFromName(name string) (gpio.PinIO, error) {
// 	pin := gpioreg.ByName(name)
// 	if pin == nil {
// 		return nil, fmt.Errorf("pin %s not found", name)
// 	}

// 	pinIO, ok := pin.(gpio.PinIO)
// 	if !ok {
// 		return nil, fmt.Errorf("pin %s does not support input/output", name)
// 	}

// 	return pinIO, nil
// }

// func resetDevice(reset gpio.PinOut) error {
// 	// Active-low reset: hold low, then release high.
// 	if err := reset.Out(gpio.Low); err != nil {
// 		return err
// 	}

// 	time.Sleep(10 * time.Millisecond)

// 	if err := reset.Out(gpio.High); err != nil {
// 		return err
// 	}

// 	time.Sleep(100 * time.Millisecond)
// 	return nil
// }

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
