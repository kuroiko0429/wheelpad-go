package main

import (
	"os"
	"testing"
)

func TestWheelSteps(t *testing.T) {
	cases := []struct {
		values, want []int32
		remainder    int32
	}{
		{[]int32{60, 60, 60, 60}, []int32{0, 1, 0, 1}, 0},
		{[]int32{-60, -60, -30}, []int32{0, -1, 0}, -30},
		{[]int32{60, -60, -120, 240}, []int32{0, 0, -1, 2}, 0},
		{[]int32{300}, []int32{2}, 60},
	}
	for _, c := range cases {
		var r int32
		for i, v := range c.values {
			if got := wheelSteps(&r, v); got != c.want[i] {
				t.Fatalf("%v at %d: got %d want %d", c.values, i, got, c.want[i])
			}
		}
		if r != c.remainder {
			t.Fatalf("remainder %d want %d", r, c.remainder)
		}
	}
}

func TestSmallMovementDoesNotStartGlide(t *testing.T) {
	engine := &InertiaEngine{enabled: true, minVelocity: 3, stopVelocity: 0.2}
	for _, velocity := range []float64{0, 0.5, -2.9, 3} {
		engine.Trigger(velocity, 1, false, 10)
		if engine.cancelFn != nil {
			t.Fatalf("velocity %v started a glide", velocity)
		}
	}
}

func TestStoppingVelocity(t *testing.T) {
	for _, c := range []struct{ stop, want float64 }{{0, 3}, {-1, 3}, {0.2, 0.2}} {
		engine := &InertiaEngine{minVelocity: 3, stopVelocity: c.stop}
		if got := engine.stoppingVelocity(); got != c.want {
			t.Fatalf("stop_velocity=%v: got %v want %v", c.stop, got, c.want)
		}
	}
}

func TestInertiaConfig(t *testing.T) {
	for _, c := range []struct {
		config string
		want   float64
	}{
		{"[inertia]\nmin_velocity = 3\n", 3},
		{"[inertia]\nmin_velocity = 3\nstop_velocity = 0.2\n", 0.2},
	} {
		path := t.TempDir() + "/config.toml"
		if err := os.WriteFile(path, []byte(c.config), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		engine := NewInertiaEngine(nil, nil, cfg.Inertia)
		if got := engine.stoppingVelocity(); got != c.want {
			t.Fatalf("got stopping velocity %v want %v", got, c.want)
		}
	}
}

func TestLaunchVelocity(t *testing.T) {
	for _, c := range []struct{ gain, want float64 }{{0, 4}, {-1, 4}, {1, 4}, {2, 8}} {
		engine := &InertiaEngine{gain: c.gain}
		if got := engine.launchVelocity(-4); got != c.want {
			t.Fatalf("gain=%v: got %v want %v", c.gain, got, c.want)
		}
	}
}
