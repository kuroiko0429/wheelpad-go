package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func captureLogs(t *testing.T, debug bool) *bytes.Buffer {
	t.Helper()
	oldOutput, oldDebug := log.Writer(), debugEnabled
	var output bytes.Buffer
	log.SetOutput(&output)
	debugEnabled = debug
	t.Cleanup(func() {
		log.SetOutput(oldOutput)
		debugEnabled = oldDebug
	})
	return &output
}

func TestFindTouchpadLogsOpenFailures(t *testing.T) {
	for _, debug := range []bool{true, false} {
		t.Run(fmt.Sprintf("debug=%v", debug), func(t *testing.T) {
			output := captureLogs(t, debug)
			path := filepath.Join(t.TempDir(), "event-missing")
			found, err := findTouchpadFromPaths("Synaptics TM3562-003", []string{path})
			if err != nil || found != "" {
				t.Fatalf("unavailable devices should be skipped: %q, %v", found, err)
			}
			if debug && (!strings.Contains(output.String(), path) || !strings.Contains(output.String(), "no such file or directory")) {
				t.Errorf("missing path and open error in debug log: %s", output)
			}
			if !debug && output.Len() != 0 {
				t.Errorf("open failures should only be logged in debug mode: %s", output)
			}
		})
	}
}

func TestFindTouchpadLogsPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can bypass file permissions")
	}
	path := filepath.Join(t.TempDir(), "event-denied")
	if err := os.WriteFile(path, nil, 0000); err != nil {
		t.Fatal(err)
	}
	output := captureLogs(t, true)
	found, err := findTouchpadFromPaths("Synaptics TM3562-003", []string{path})
	if err != nil || found != "" {
		t.Fatalf("inaccessible devices should be skipped: %q, %v", found, err)
	}
	if !strings.Contains(output.String(), path) || !strings.Contains(output.String(), "permission denied") {
		t.Errorf("permission error was swallowed: %s", output)
	}
}

func TestInitializationErrorHints(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"busy", fmt.Errorf("grab touchpad: %w", syscall.EBUSY), "他のプロセス"},
		{"busy", errors.New(syscall.EBUSY.Error()), "他のプロセス"},
		{"busy", fmt.Errorf("grab touchpad: %w", errors.New(syscall.EBUSY.Error())), "他のプロセス"},
		{"access", &os.PathError{Op: "open", Path: "/dev/uinput", Err: syscall.EACCES}, "アクセス権"},
		{"access", errors.New(syscall.EACCES.Error()), "アクセス権"},
		{"permission", fmt.Errorf("create device: %w", syscall.EPERM), "アクセス権"},
		{"permission", errors.New(syscall.EPERM.Error()), "アクセス権"},
		{"other", errors.New("unexpected failure"), "デーモンの初期化に失敗しました"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureLogs(t, false)
			logInitializationError(tc.err)
			message := output.String()
			if !strings.Contains(message, tc.want) || !strings.Contains(message, tc.err.Error()) {
				t.Errorf("missing diagnosis or original error: %s", message)
			}
			if tc.name == "busy" && strings.Contains(message, "アクセス権") {
				t.Errorf("busy device was misdiagnosed as a permission error: %s", message)
			}
			if tc.name == "other" && strings.Contains(message, "アクセス権") {
				t.Errorf("unknown failure was misdiagnosed as a permission error: %s", message)
			}
		})
	}
}
