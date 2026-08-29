package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"periph.io/x/conn/v3/gpio"
)

const (
	SLOW = 1 * time.Second
	FAST = 4 * time.Second
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

const (
	RED   = "GPIO23"
	GREEN = "GPIO17"
)

/******************* Usage **********************/
//	redLight := NewGPIOLED("Red Light", RED)
//	greenLight := NewGPIOLED("Green Light", GREEN)
//	leds := InitLEDController(redLight, greenLight)
//  leds.SetState(REDON)
//  leds.SetState(GREENSLOW)
//  leds.SetState(OFF)

// LEDController is the singleton responsible for controlling both LEDs.
type LEDController struct {
	red   *LEDGPIO
	green *LEDGPIO

	mu           sync.Mutex
	currentState int
	cancelBlink  context.CancelFunc
	blinkWG      sync.WaitGroup
}

var (
	controller     *LEDController
	controllerOnce sync.Once
)

// InitLEDController initializes the singleton exactly once.
//
// Call this after creating/opening the red and green LEDGPIO values.
func InitLEDController(red, green *LEDGPIO) *LEDController {
	controllerOnce.Do(func() {
		controller = &LEDController{
			red:          red,
			green:        green,
			currentState: OFF,
		}
	})

	return controller
}

// LEDControllerInstance returns the initialized singleton.
func LEDControllerInstance() *LEDController {
	if controller == nil {
		panic("LEDController has not been initialized; call InitLEDController first")
	}

	return controller
}

// SetState changes the LED state.
//
// Valid states are:
// OFF, REDON, GREENON, REDSLOW, GREENSLOW, REDFAST, GREENFAST.
func (c *LEDController) SetState(newState int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if newState == c.currentState {
		return
	}

	// Stop any existing blinking goroutine before changing the LEDs.
	c.stopBlinkLocked()

	// Always turn both LEDs off before applying the new state.
	c.red.Off()
	c.green.Off()

	switch newState {
	case OFF:
		// Both LEDs are already off.

	case REDON:
		c.red.On()

	case GREENON:
		c.green.On()

	case REDSLOW:
		c.startBlinkLocked(c.red, SLOW)

	case GREENSLOW:
		c.startBlinkLocked(c.green, SLOW)

	case REDFAST:
		c.startBlinkLocked(c.red, FAST)

	case GREENFAST:
		c.startBlinkLocked(c.green, FAST)

	default:
		logger.Error("Invalid LED state", "state", newState)
		return
	}

	c.currentState = newState
}

// startBlinkLocked starts blinking one LED.
//
// The caller must hold c.mu.
func (c *LEDController) startBlinkLocked(led *LEDGPIO, interval time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelBlink = cancel

	c.blinkWG.Add(1)
	go func() {
		defer c.blinkWG.Done()
		blink(led, ctx, interval)
	}()
}

// stopBlinkLocked stops the current blinking goroutine.
//
// The caller must hold c.mu.
func (c *LEDController) stopBlinkLocked() {
	if c.cancelBlink == nil {
		return
	}

	c.cancelBlink()
	c.cancelBlink = nil

	// Wait until the previous blinking goroutine has exited.
	c.blinkWG.Wait()
}

// Close stops blinking and turns both LEDs off.
func (c *LEDController) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopBlinkLocked()

	c.red.Off()
	c.green.Off()
	c.currentState = OFF
}

// blink runs in a separate goroutine.
func blink(led *LEDGPIO, ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Debug(fmt.Sprintf("Started blinking for %s.", led.Name))

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
