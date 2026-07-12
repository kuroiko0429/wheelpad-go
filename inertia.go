package main

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/holoplot/go-evdev"
)

// InertiaEngine simulates decay/inertia scroll when finger is lifted.
type InertiaEngine struct {
	vScroll     *evdev.InputDevice
	writeMutex  *sync.Mutex
	friction    float64
	minVelocity float64
	interval    time.Duration
	enabled     bool

	mu       sync.Mutex
	cancelFn context.CancelFunc
}

// NewInertiaEngine creates a new InertiaEngine.
func NewInertiaEngine(vScroll *evdev.InputDevice, writeMutex *sync.Mutex, cfg InertiaConfig) *InertiaEngine {
	return &InertiaEngine{
		vScroll:     vScroll,
		writeMutex:  writeMutex,
		friction:    cfg.Friction,
		minVelocity: cfg.MinVelocity,
		interval:    time.Duration(cfg.Interval * float64(time.Second)),
		enabled:     cfg.Enabled,
	}
}

// Trigger starts the inertia scrolling loop. If an existing loop is active, it gets canceled first.
func (ie *InertiaEngine) Trigger(angularVelocity float64, direction int32, horizontal bool, hiresStep int32) {
	if !ie.enabled {
		return
	}

	ie.Stop()

	ie.mu.Lock()
	defer ie.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	ie.cancelFn = cancel

	go ie.run(ctx, math.Abs(angularVelocity), direction, horizontal, hiresStep)
}

// Stop cancels any active inertia scrolling loop.
func (ie *InertiaEngine) Stop() {
	ie.mu.Lock()
	defer ie.mu.Unlock()

	if ie.cancelFn != nil {
		ie.cancelFn()
		ie.cancelFn = nil
	}
}

// run performs the periodic scroll events, slowing down over time.
func (ie *InertiaEngine) run(ctx context.Context, velocity float64, direction int32, horizontal bool, hiresStep int32) {
	ticker := time.NewTicker(ie.interval)
	defer ticker.Stop()

	for velocity > ie.minVelocity {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hiresValue := direction * int32(float64(hiresStep)*(velocity/2.0))
			if abs(hiresValue) < 1 {
				hiresValue = direction
			}

			ie.writeMutex.Lock()
			if horizontal {
				ie.vScroll.WriteOne(&evdev.InputEvent{
					Type:  evdev.EV_REL,
					Code:  evdev.REL_HWHEEL,
					Value: direction,
				})
				ie.vScroll.WriteOne(&evdev.InputEvent{
					Type:  evdev.EV_REL,
					Code:  evdev.REL_HWHEEL_HI_RES,
					Value: hiresValue,
				})
			} else {
				ie.vScroll.WriteOne(&evdev.InputEvent{
					Type:  evdev.EV_REL,
					Code:  evdev.REL_WHEEL,
					Value: direction,
				})
				ie.vScroll.WriteOne(&evdev.InputEvent{
					Type:  evdev.EV_REL,
					Code:  evdev.REL_WHEEL_HI_RES,
					Value: hiresValue,
				})
			}
			ie.vScroll.WriteOne(&evdev.InputEvent{
				Type:  evdev.EV_SYN,
				Code:  evdev.SYN_REPORT,
				Value: 0,
			})
			ie.writeMutex.Unlock()

			velocity *= ie.friction
		}
	}
}

// abs is a simple helper returning absolute value of an int32.
func abs(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}
