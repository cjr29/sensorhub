package main

import (
	"context"
	"fmt"
	"time"

	"periph.io/x/conn/v3/gpio"
)

const (
	SLOW = 1
	FAST = 4
)

const (
	OFF int = iota
	REDON
	GREENON
	REDSLOW
	GREENSLOW
	REDFAST
	GREENFAST
)

var (
	controller *LightController
	stopBlink  context.CancelFunc
	ctxBlink   context.Context
)

// Create activity Light and other layout status lights and button
var redLight LEDGPIO
var greenLight LEDGPIO
var leds LightController

// LightController is the singleton responsible for controlling both LEDs.
type LightController struct {
	red          *LEDGPIO
	green        *LEDGPIO
	currentState int
}

// InitLightController initializes the singleton exactly once.
//
// Call this after creating/opening the red and green LEDGPIO values.
func initLightController(red, green *LEDGPIO) *LightController {
	controller = &LightController{
		red:          red,
		green:        green,
		currentState: OFF,
	}
	ctxBlink, stopBlink = context.WithCancel(context.Background())
	return controller
}

// LEDControllerInstance returns the initialized singleton.
func LightControllerInstance() *LightController {
	if controller == nil {
		panic("LightController has not been initialized; call InitLightController first")
	}
	return controller
}

/*********** Main entry to goroutine that controls activity LEDs ***************/
/*                   lightControl                                              */
/*******************************************************************************/
func lightControl(ctx context.Context, cmd chan int) {
	var redLight = NewGPIOLED("Red Light", RED)
	var greenLight = NewGPIOLED("Green Light", GREEN)
	var leds = initLightController(redLight, greenLight)
	var currentState int = OFF

	logger.Debug("LED controller and red/green lights configured.")

	// Run a loop waiting for a channel command for lights
	// commands: OFF, GREENON, REDON, GREENSLOW, GREENFAST, REDSLOW, REDFAST
	// If done context command received, shuts off bloth leds and returns

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
				if currentState == REDSLOW || currentState == REDFAST || currentState == GREENSLOW || currentState == GREENFAST {
					stopBlink()
				}
				switch currentState {
				case OFF:
					leds.red.Off()
					leds.green.Off()
					logger.Debug("SetState to OFF")
				case REDON:
					leds.red.On()
					logger.Debug("SetState to REDON")

				case GREENON:
					leds.green.On()
					logger.Debug("SetState to GREENON")

				case REDSLOW:
					go blink(leds.red, ctxBlink, SLOW)
					logger.Debug("SetState to REDSLOW")

				case GREENSLOW:
					go blink(leds.green, ctxBlink, SLOW)
					logger.Debug("SetState to GREENSLOW")

				case REDFAST:
					go blink(leds.red, ctxBlink, FAST)
					logger.Debug("SetState to REDFAST")

				case GREENFAST:
					go blink(leds.green, ctxBlink, FAST)
					logger.Debug("SetState to GREENFAST")

				default:
					logger.Error("Invalid LED state", "state", newState)
					return
				}
			}
		}
	}
}

// blink runs in a separate goroutine.
func blink(led *LEDGPIO, ctx context.Context, rateHz float64) {

	logger.Debug(fmt.Sprintf("Started blinking for %s.", led.Name))

	if rateHz <= 0 {
		return
	}

	// One complete cycle consists of an ON interval and an OFF interval.
	halfPeriod := time.Duration(float64(time.Second) / (2 * rateHz))
	if halfPeriod <= 0 {
		return
	}

	ticker := time.NewTicker(halfPeriod)
	defer ticker.Stop()

	led.Off()
	led.State = false

	for {
		select {
		case <-ctx.Done():
			led.Off()
			// logger.Debug(fmt.Sprintf("Blinking stopped for %s.", led.Name))
			return

		case <-ticker.C:
			led.Toggle()
		}
	}
}

func (c *LightController) Close() {
	stopBlink()
	releasePin(c.red.Pin)
	releasePin(c.green.Pin)
}

func releasePin(pin gpio.PinOut) {
	if err := pin.Halt(); err != nil {
		logger.Debug(fmt.Sprintf("failed to release GPIO: %v", err))
	}
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
