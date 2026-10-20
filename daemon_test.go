package main

import (
	"math"
	"testing"
	"time"

	"github.com/holoplot/go-evdev"
)

func TestReleaseVelocity(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name string
		age  time.Duration
		want float64
	}{
		{"immediate flick", 10 * time.Millisecond, 8},
		{"recent movement", 99 * time.Millisecond, 8},
		{"pause boundary", 100 * time.Millisecond, 0},
		{"rested finger", time.Second, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := WheelpadDaemon{lastMoveTime: now.Add(-tc.age), lastAngularVelocity: 8}
			if got := d.releaseVelocity(now); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStoppedFingerReleaseDoesNotStartInertia(t *testing.T) {
	engine := &InertiaEngine{enabled: true, minVelocity: 3}
	d := WheelpadDaemon{
		inertia:             engine,
		scrollMode:          true,
		isTouching:          true,
		lastMoveTime:        time.Now().Add(-time.Second),
		lastAngularVelocity: 8,
		buffer:              []evdev.InputEvent{{Type: evdev.EV_KEY, Code: evdev.BTN_TOUCH, Value: 0}},
	}
	if !d.detectTouchChange() {
		t.Fatal("release was not consumed")
	}
	if engine.cancelFn != nil {
		t.Fatal("rested finger started inertia")
	}
	if d.scrollMode || d.isTouching {
		t.Fatal("release did not end scroll gesture")
	}
}

// Movement below the scroll sensitivity emits no step but still counts as motion,
// while resting-finger jitter does not.
func TestSubStepMovementKeepsReleaseVelocity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		angle float64
		want  float64
	}{
		{"slow continuous movement", 0.1, 2},
		{"resting finger jitter", 0.005, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stale := time.Now().Add(-120 * time.Millisecond)
			d := WheelpadDaemon{
				cfgWp:               WheelpadConfig{Sensitivity: 0.3},
				hasLastAngle:        true,
				lastTime:            stale,
				lastMoveTime:        stale,
				lastAngularVelocity: 2,
				currentX:            int(math.Round(1000 * math.Cos(tc.angle))),
				currentY:            int(math.Round(1000 * math.Sin(tc.angle))),
			}
			d.handleScroll()
			if got := d.releaseVelocity(time.Now()); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
