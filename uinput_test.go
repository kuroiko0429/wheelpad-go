package main

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestDoIoctlPreservesErrno(t *testing.T) {
	// ioctl arguments may be integer values, not memory addresses.
	if err := doIoctl(^uintptr(0), 0, uintptr(1)); !errors.Is(err, syscall.EBADF) {
		t.Fatalf("invalid descriptor: got %v, want EBADF", err)
	}
}

func TestUinputBitIoctls(t *testing.T) {
	if os.Getenv("WHEELPAD_TEST_UINPUT") != "1" {
		t.Skip("set WHEELPAD_TEST_UINPUT=1 to configure an isolated uinput descriptor")
	}
	// Configure only: never create a device, grab physical input or send events.
	file, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, tc := range []struct {
		name string
		set  func(uintptr, uintptr) error
		bit  uintptr
	}{
		{"event", ioctlUISETEVBIT, 1}, // EV_KEY
		{"key", ioctlUISETKEYBIT, 30}, // KEY_A
		{"relative", ioctlUISETRELBIT, 0},
		{"absolute", ioctlUISETABSBIT, 0},
		{"misc", ioctlUISETMSCBIT, 0},
		{"led", ioctlUISETLEDBIT, 0},
		{"sound", ioctlUISETSNDBIT, 0},
		{"force-feedback", ioctlUISETFFBIT, 0x50}, // FF_RUMBLE
		{"switch", ioctlUISETSWBIT, 0},
		{"property", ioctlUISETPROPBIT, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.set(file.Fd(), tc.bit); err != nil {
				t.Fatalf("set bit %d: %v", tc.bit, err)
			}
		})
	}
}
