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
	vScroll      *evdev.InputDevice
	writeMutex   *sync.Mutex
	friction     float64
	minVelocity  float64
	stopVelocity float64
	gain         float64
	interval     time.Duration
	enabled      bool

	mu        sync.Mutex
	cancelFn  context.CancelFunc
	remainder [2]int32
}

// NewInertiaEngine creates a new InertiaEngine.
func NewInertiaEngine(vScroll *evdev.InputDevice, writeMutex *sync.Mutex, cfg InertiaConfig) *InertiaEngine {
	return &InertiaEngine{
		vScroll:      vScroll,
		writeMutex:   writeMutex,
		friction:     cfg.Friction,
		minVelocity:  cfg.MinVelocity,
		stopVelocity: cfg.StopVelocity,
		gain:         cfg.Gain,
		interval:     time.Duration(cfg.Interval * float64(time.Second)),
		enabled:      cfg.Enabled,
	}
}

// Trigger starts the inertia scrolling loop. If an existing loop is active, it gets canceled first.
func (ie *InertiaEngine) Trigger(angularVelocity float64, direction int32, horizontal bool, hiresStep int32) {
	if !ie.enabled {
		return
	}

	ie.Stop()

	if math.Abs(angularVelocity) <= ie.minVelocity {
		return
	}

	ie.mu.Lock()
	defer ie.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	ie.cancelFn = cancel

	go ie.run(ctx, ie.launchVelocity(angularVelocity), direction, horizontal, hiresStep)
}

// launchVelocity scales the glide speed; configs without gain keep the release velocity.
func (ie *InertiaEngine) launchVelocity(angularVelocity float64) float64 {
	velocity := math.Abs(angularVelocity)
	if ie.gain > 0 {
		velocity *= ie.gain
	}
	return velocity
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

// stoppingVelocity keeps configs without stop_velocity compatible.
func (ie *InertiaEngine) stoppingVelocity() float64 {
	if ie.stopVelocity > 0 {
		return ie.stopVelocity
	}
	return ie.minVelocity
}

// run performs the periodic scroll events, slowing down over time.
func (ie *InertiaEngine) run(ctx context.Context, velocity float64, direction int32, horizontal bool, hiresStep int32) {
	ticker := time.NewTicker(ie.interval)
	defer ticker.Stop()

	// Older configs retain their original stopping threshold.
	stopVelocity := ie.stoppingVelocity()
	for velocity > stopVelocity {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hiresValue := direction * int32(float64(hiresStep)*(velocity/2.0))
			if abs(hiresValue) < 1 {
				hiresValue = direction
			}

			ie.writeScroll(hiresValue, horizontal)

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

// wheelSteps keeps legacy detents consistent with high-resolution units (120 per detent).
func wheelSteps(remainder *int32, value int32) int32 {
	*remainder += value
	steps := *remainder / 120
	*remainder -= steps * 120
	return steps
}

func (ie *InertiaEngine) writeScroll(value int32, horizontal bool) {
	ie.writeMutex.Lock()
	defer ie.writeMutex.Unlock()
	axis := 0
	legacy, hires := evdev.EvCode(evdev.REL_WHEEL), evdev.EvCode(evdev.REL_WHEEL_HI_RES)
	if horizontal {
		axis = 1
		legacy, hires = evdev.REL_HWHEEL, evdev.REL_HWHEEL_HI_RES
	}
	steps := wheelSteps(&ie.remainder[axis], value)
	if steps != 0 {
		ie.vScroll.WriteOne(&evdev.InputEvent{Type: evdev.EV_REL, Code: legacy, Value: steps})
	}
	ie.vScroll.WriteOne(&evdev.InputEvent{Type: evdev.EV_REL, Code: hires, Value: value})
	ie.vScroll.WriteOne(&evdev.InputEvent{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT})
}
