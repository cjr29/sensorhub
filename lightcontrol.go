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
	stopLight  context.CancelFunc
)

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
	stopLight = nil
	return controller
}

// LEDControllerInstance returns the initialized singleton.
func LightControllerInstance() *LightController {
	if controller == nil {
		panic("LightController has not been initialized; call InitLightController first")
	}
	return controller
}

// SetState changes the LED state.
//
// Valid states are:
// OFF, REDON, GREENON, REDSLOW, GREENSLOW, REDFAST, GREENFAST.
func (c *LightController) SetState(newState int) {
	if newState == c.currentState {
		return
	}
	// Stop any existing blinking goroutine and turn off LEDs before changing the LEDs.
	c.Off()

	switch newState {
	case OFF:
		// Both LEDs are already off.

	case REDON:
		c.red.On()

	case GREENON:
		c.green.On()

	case REDSLOW:
		c.startBlink(c.red, SLOW)

	case GREENSLOW:
		c.startBlink(c.green, SLOW)

	case REDFAST:
		c.startBlink(c.red, FAST)

	case GREENFAST:
		c.startBlink(c.green, FAST)

	default:
		logger.Error("Invalid LED state", "state", newState)
		return
	}

	logger.Debug(fmt.Sprintf("LED state: %v", newState))
	c.currentState = newState
}

func (c *LightController) startBlink(led *LEDGPIO, rateHz float64) {
	if stopLight != nil {
		stopLight()
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopLight = cancel
	go blink(led, ctx, rateHz)
}

// Close stops blinking and turns both LEDs off.
func (c *LightController) Off() {
	if stopLight != nil {
		stopLight()
		stopLight = nil
	}
	c.red.Off()
	c.green.Off()
	c.currentState = OFF
}

// Close stops blinking and turns both LEDs off.
func (c *LightController) Close() {
	c.Off()
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
			logger.Debug(fmt.Sprintf("Blinking stopped for %s.", led.Name))
			return

		case <-ticker.C:
			led.Toggle()
		}
	}
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
