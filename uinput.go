package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/holoplot/go-evdev"
)

const (
	uinputMaxNameSize = 80
	absSize           = 64
)

// uinputUserDevice matches the C struct uinput_user_device / evdev.UinputUserDevice layout
type uinputUserDevice struct {
	Name       [uinputMaxNameSize]byte
	ID         evdev.InputID
	EffectsMax uint32
	Absmax     [absSize]int32
	Absmin     [absSize]int32
	Absfuzz    [absSize]int32
	Absflat    [absSize]int32
}

// VirtualDevice wraps the newly created /dev/uinput descriptor to write events
type VirtualDevice struct {
	file *os.File
}

// WriteOne writes a single input event to the virtual device descriptor
func (vd *VirtualDevice) WriteOne(event *evdev.InputEvent) error {
	return binary.Write(vd.file, binary.LittleEndian, event)
}

// Close destroys the uinput virtual device and closes the file descriptor
func (vd *VirtualDevice) Close() error {
	_ = ioctlUIDEVDESTROY(vd.file.Fd())
	return vd.file.Close()
}

// Raw ioctl helpers for uinput configuration
const (
	ioctlDirNone  = 0x0
	ioctlDirWrite = 0x1
	ioctlDirRead  = 0x2
)

func ioctlMakeCode(dir, typ, nr int, size uintptr) uint32 {
	var code uint32
	code |= uint32(dir) << 30
	code |= uint32(size) << 16
	code |= uint32(typ) << 8
	code |= uint32(nr)
	return code
}

func doIoctl(fd uintptr, code uint32, ptr unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(code), uintptr(ptr))
	if errno != 0 {
		return errno
	}
	return nil
}

func ioctlUISETEVBIT(fd uintptr, ev uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 100, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(ev))
}

func ioctlUISETKEYBIT(fd uintptr, key uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 101, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(key))
}

func ioctlUISETRELBIT(fd uintptr, rel uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 102, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(rel))
}

func ioctlUISETABSBIT(fd uintptr, abs uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 103, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(abs))
}

func ioctlUISETMSCBIT(fd uintptr, msc uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 104, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(msc))
}

func ioctlUISETLEDBIT(fd uintptr, led uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 105, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(led))
}

func ioctlUISETSNDBIT(fd uintptr, snd uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 106, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(snd))
}

func ioctlUISETFFBIT(fd uintptr, fe uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 107, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(fe))
}

func ioctlUISETSWBIT(fd uintptr, sw uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 109, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(sw))
}

func ioctlUISETPROPBIT(fd uintptr, prop uintptr) error {
	var p int32
	code := ioctlMakeCode(ioctlDirWrite, 'U', 110, unsafe.Sizeof(p))
	return doIoctl(fd, code, unsafe.Pointer(prop))
}

func ioctlUIDEVCREATE(fd uintptr) error {
	code := ioctlMakeCode(ioctlDirNone, 'U', 1, 0)
	return doIoctl(fd, code, nil)
}

func ioctlUIDEVDESTROY(fd uintptr) error {
	code := ioctlMakeCode(ioctlDirNone, 'U', 2, 0)
	return doIoctl(fd, code, nil)
}

// CloneDeviceCustom mirrors the physical device's exact keys, types, and ABS axis ranges.
func CloneDeviceCustom(name string, dev *evdev.InputDevice) (*VirtualDevice, error) {
	deviceFile, err := os.OpenFile("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK, 0660)
	if err != nil {
		return nil, fmt.Errorf("failed to open /dev/uinput: %w", err)
	}

	fd := deviceFile.Fd()

	// 1. Enable capable event types & codes
	for _, ev := range dev.CapableTypes() {
		// Do not set EV_SYN manually; uinput handles it automatically
		if ev == 0 { // EV_SYN is 0
			continue
		}

		if err := ioctlUISETEVBIT(fd, uintptr(ev)); err != nil {
			deviceFile.Close()
			return nil, fmt.Errorf("failed to set ev bit %d: %w", ev, err)
		}

		for _, code := range dev.CapableEvents(ev) {
			var err error
			switch ev {
			case 0x01: // EV_KEY
				err = ioctlUISETKEYBIT(fd, uintptr(code))
			case 0x02: // EV_REL
				err = ioctlUISETRELBIT(fd, uintptr(code))
			case 0x03: // EV_ABS
				err = ioctlUISETABSBIT(fd, uintptr(code))
			case 0x04: // EV_MSC
				err = ioctlUISETMSCBIT(fd, uintptr(code))
			case 0x05: // EV_SW
				err = ioctlUISETSWBIT(fd, uintptr(code))
			case 0x11: // EV_LED
				err = ioctlUISETLEDBIT(fd, uintptr(code))
			case 0x12: // EV_SND
				err = ioctlUISETSNDBIT(fd, uintptr(code))
			case 0x15: // EV_FF
				err = ioctlUISETFFBIT(fd, uintptr(code))
			}
			if err != nil {
				deviceFile.Close()
				return nil, fmt.Errorf("failed to set event code %d for type %d: %w", code, ev, err)
			}
		}
	}

	// 2. Enable properties (e.g. INPUT_PROP_BUTTONPAD)
	for _, prop := range dev.Properties() {
		if err := ioctlUISETPROPBIT(fd, uintptr(prop)); err != nil {
			deviceFile.Close()
			return nil, fmt.Errorf("failed to set prop bit %d: %w", prop, err)
		}
	}

	// 3. Retrieve physical InputID
	id, err := dev.InputID()
	if err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to get physical device ID: %w", err)
	}

	// 4. Populate uinputUserDevice structure with proper ABS ranges
	var uidev uinputUserDevice
	copy(uidev.Name[:], []byte(name))
	uidev.ID = id

	absInfos, err := dev.AbsInfos()
	if err == nil {
		for code, info := range absInfos {
			if code < absSize {
				uidev.Absmax[code] = info.Maximum
				uidev.Absmin[code] = info.Minimum
				uidev.Absfuzz[code] = info.Fuzz
				uidev.Absflat[code] = info.Flat
			}
		}
	}

	// 5. Serialize and write the device definition struct to uinput
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, uidev); err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to serialize uidev: %w", err)
	}

	if _, err := deviceFile.Write(buf.Bytes()); err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to write uidev struct to /dev/uinput: %w", err)
	}

	// 6. Request device creation
	if err := ioctlUIDEVCREATE(fd); err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to invoke UI_DEV_CREATE: %w", err)
	}

	return &VirtualDevice{file: deviceFile}, nil
}
