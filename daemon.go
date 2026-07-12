package main

import (
	"math"
	"sync"
	"time"

	"github.com/holoplot/go-evdev"
)

type WheelpadDaemon struct {
	cfgWp    WheelpadConfig
	cfgSpeed SpeedConfig

	pad     *evdev.InputDevice
	vPad    *VirtualDevice
	vScroll *evdev.InputDevice
	inertia *InertiaEngine

	writeMutex *sync.Mutex

	buffer []evdev.InputEvent

	isTouching bool
	scrollMode bool

	fingerCount int
	hasLastAngle bool
	lastAngle    float64
	lastTime     time.Time

	lastDirection       int32
	lastHorizontal      bool
	lastAngularVelocity float64

	currentX int
	currentY int

	scrollSign int32
}

// NewWheelpadDaemon instantiates the WheelpadDaemon, establishing grabs and virtual devices.
func NewWheelpadDaemon(devicePath string, cfg Config) (*WheelpadDaemon, error) {
	pad, err := evdev.Open(devicePath)
	if err != nil {
		return nil, err
	}

	err = pad.Grab()
	if err != nil {
		pad.Close()
		return nil, err
	}

	// Virtual touchpad for forwarding normal events (copies physical device capabilities)
	vPad, err := CloneDeviceCustom("LetsNote-Virtual-Pad", pad)
	if err != nil {
		pad.Ungrab()
		pad.Close()
		return nil, err
	}

	// Capabilities for virtual scroll: relative vertical and horizontal scroll
	scrollCapabilities := map[evdev.EvType][]evdev.EvCode{
		evdev.EV_REL: {
			evdev.REL_WHEEL,
			evdev.REL_WHEEL_HI_RES,
			evdev.REL_HWHEEL,
			evdev.REL_HWHEEL_HI_RES,
		},
	}

	scrollID := evdev.InputID{
		BusType: 0x03, // USB or virtual standard
		Vendor:  0x0001,
		Product: 0x0001,
		Version: 1,
	}

	vScroll, err := evdev.CreateDevice("LetsNote-Virtual-Wheel", scrollID, scrollCapabilities)
	if err != nil {
		vPad.Close()
		pad.Ungrab()
		pad.Close()
		return nil, err
	}

	writeMutex := &sync.Mutex{}
	inertia := NewInertiaEngine(vScroll, writeMutex, cfg.Inertia)

	var scrollSign int32 = 1
	if cfg.Wheelpad.NaturalScroll {
		scrollSign = -1
	}

	daemon := &WheelpadDaemon{
		cfgWp:    cfg.Wheelpad,
		cfgSpeed: cfg.Speed,

		pad:     pad,
		vPad:    vPad,
		vScroll: vScroll,
		inertia: inertia,

		writeMutex: writeMutex,

		buffer: make([]evdev.InputEvent, 0, 128),

		isTouching: false,
		scrollMode: false,

		fingerCount: 0,
		hasLastAngle: false,

		lastDirection:       1,
		lastHorizontal:      false,
		lastAngularVelocity: 0.0,

		currentX: cfg.Wheelpad.CenterX,
		currentY: cfg.Wheelpad.CenterY,

		scrollSign: scrollSign,
	}

	name, _ := pad.Name()
	logInfo("デーモン起動: %s (%s)", name, devicePath)
	return daemon, nil
}

// Cleanup releases grabs and closes both virtual and physical devices.
func (d *WheelpadDaemon) Cleanup() {
	d.inertia.Stop()
	d.pad.Ungrab()
	d.vPad.Close()
	d.vScroll.Close()
	d.pad.Close()
}

// Run starts the main read loop.
func (d *WheelpadDaemon) Run() error {
	for {
		event, err := d.pad.ReadOne()
		if err != nil {
			return err
		}

		// Copy the struct value to prevent pointer/buffer reuse bugs in evdev
		d.buffer = append(d.buffer, *event)
		d.updatePosition(event)

		if event.Type == evdev.EV_SYN && event.Code == evdev.SYN_REPORT {
			d.processFrame()
			d.buffer = d.buffer[:0]
		}
	}
}

func (d *WheelpadDaemon) updatePosition(event *evdev.InputEvent) {
	if event.Type != evdev.EV_ABS {
		return
	}
	if event.Code == evdev.ABS_X || event.Code == evdev.ABS_MT_POSITION_X {
		d.currentX = int(event.Value)
	} else if event.Code == evdev.ABS_Y || event.Code == evdev.ABS_MT_POSITION_Y {
		d.currentY = int(event.Value)
	}
}

func (d *WheelpadDaemon) processFrame() {
	if d.detectTouchChange() {
		return
	}
	d.detectFingerCount()

	if d.scrollMode && d.isTouching {
		d.handleScroll()
	} else if !d.scrollMode {
		d.forwardEvents()
	}
}

func (d *WheelpadDaemon) detectTouchChange() bool {
	cx := float64(d.cfgWp.CenterX)
	cy := float64(d.cfgWp.CenterY)

	for _, e := range d.buffer {
		if e.Type == evdev.EV_KEY && e.Code == evdev.BTN_TOUCH {
			d.isTouching = e.Value == 1
			logDebug("BTN_TOUCH 変化: isTouching=%v", d.isTouching)

			if d.isTouching {
				// Cancel any active inertia when a new touch begins
				d.inertia.Stop()

				dist := math.Hypot(float64(d.currentX)-cx, float64(d.currentY)-cy)
				logDebug("タッチ開始座標: X=%d, Y=%d, 中心距離=%.2f (デッドゾーン=%.2f)", d.currentX, d.currentY, dist, d.cfgWp.Deadzone)
				if dist > d.cfgWp.Deadzone {
					d.scrollMode = true
					d.lastAngle = math.Atan2(float64(d.currentY)-cy, float64(d.currentX)-cx)
					d.hasLastAngle = true
					d.lastTime = time.Now()
					d.lastAngularVelocity = 0.0
					logDebug("スクロールモード開始")
				} else {
					d.scrollMode = false
					logDebug("ノーマルモード開始 (デッドゾーン内)")
				}
			} else {
				logDebug("タッチ終了: scrollMode=%v", d.scrollMode)
				if d.scrollMode {
					logDebug("スクロールモード終了、慣性トリガー: velocity=%.2f", d.lastAngularVelocity)
					// Trigger inertia scrolling when fingers are lifted from scroll mode
					d.inertia.Trigger(
						d.lastAngularVelocity,
						d.lastDirection,
						d.lastHorizontal,
						d.cfgWp.HiresStep,
					)
					d.scrollMode = false
					d.hasLastAngle = false
					d.buffer = d.buffer[:0]
					return true
				}
				d.scrollMode = false
			}
		}
	}
	return false
}

func (d *WheelpadDaemon) detectFingerCount() {
	for _, e := range d.buffer {
		if e.Type == evdev.EV_KEY {
			if e.Code == evdev.BTN_TOOL_FINGER {
				if e.Value != 0 {
					d.fingerCount = 1
				} else {
					d.fingerCount = 0
				}
			} else if e.Code == evdev.BTN_TOOL_DOUBLETAP {
				if e.Value != 0 {
					d.fingerCount = 2
				}
			}
		}
	}
}

func (d *WheelpadDaemon) handleScroll() {
	cx := float64(d.cfgWp.CenterX)
	cy := float64(d.cfgWp.CenterY)
	currentAngle := math.Atan2(float64(d.currentY)-cy, float64(d.currentX)-cx)
	now := time.Now()

	if !d.hasLastAngle {
		d.lastAngle = currentAngle
		d.hasLastAngle = true
		d.lastTime = now
		return
	}

	angleDiff := normalizeAngle(currentAngle - d.lastAngle)

	if math.Abs(angleDiff) < d.cfgWp.Sensitivity {
		return
	}

	dt := now.Sub(d.lastTime).Seconds()
	var angularVelocity float64
	if dt > 0 {
		angularVelocity = math.Abs(angleDiff) / dt
	}

	multiplier := calcSpeedMultiplier(angularVelocity, d.cfgSpeed.Thresholds)

	var rawDirection int32 = 1
	if angleDiff > 0 {
		rawDirection = -1
	}

	direction := rawDirection * d.scrollSign
	hiresValue := direction * d.cfgWp.HiresStep * multiplier
	horizontal := d.fingerCount >= 2

	d.lastDirection = direction
	d.lastHorizontal = horizontal
	d.lastAngularVelocity = angularVelocity

	d.writeMutex.Lock()
	if horizontal {
		d.vScroll.WriteOne(&evdev.InputEvent{
			Type:  evdev.EV_REL,
			Code:  evdev.REL_HWHEEL,
			Value: direction * multiplier,
		})
		d.vScroll.WriteOne(&evdev.InputEvent{
			Type:  evdev.EV_REL,
			Code:  evdev.REL_HWHEEL_HI_RES,
			Value: hiresValue,
		})
		logDebug("水平: dir=%d mult=%d vel=%.2f", direction, multiplier, angularVelocity)
	} else {
		d.vScroll.WriteOne(&evdev.InputEvent{
			Type:  evdev.EV_REL,
			Code:  evdev.REL_WHEEL,
			Value: direction * multiplier,
		})
		d.vScroll.WriteOne(&evdev.InputEvent{
			Type:  evdev.EV_REL,
			Code:  evdev.REL_WHEEL_HI_RES,
			Value: hiresValue,
		})
		logDebug("垂直: dir=%d mult=%d vel=%.2f", direction, multiplier, angularVelocity)
	}
	d.vScroll.WriteOne(&evdev.InputEvent{
		Type:  evdev.EV_SYN,
		Code:  evdev.SYN_REPORT,
		Value: 0,
	})
	d.writeMutex.Unlock()

	d.lastAngle = currentAngle
	d.lastTime = now
}

func (d *WheelpadDaemon) forwardEvents() {
	for _, e := range d.buffer {
		// Forward regular touchpad inputs to the virtual touchpad
		d.vPad.WriteOne(&e)
	}
}

func normalizeAngle(diff float64) float64 {
	if diff > math.Pi {
		diff -= 2 * math.Pi
	} else if diff < -math.Pi {
		diff += 2 * math.Pi
	}
	return diff
}

func calcSpeedMultiplier(angularVelocity float64, thresholds []Threshold) int32 {
	for _, entry := range thresholds {
		if angularVelocity >= entry.Velocity {
			return entry.Multiplier
		}
	}
	return 1
}
