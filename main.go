package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/holoplot/go-evdev"
)

var (
	debugEnabled bool
)

// Logging utilities matching Python's logging style.
func logInfo(format string, v ...interface{}) {
	log.Printf("[INFO] "+format, v...)
}

func logWarn(format string, v ...interface{}) {
	log.Printf("[WARNING] "+format, v...)
}

func logError(format string, v ...interface{}) {
	log.Printf("[ERROR] "+format, v...)
}

func logDebug(format string, v ...interface{}) {
	if debugEnabled {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// findTouchpad scans all event devices to find one containing the specified device name.
func findTouchpad(deviceName string) (string, error) {
	files, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return "", err
	}

	for _, f := range files {
		dev, err := evdev.Open(f)
		if err != nil {
			continue
		}
		name, err := dev.Name()
		if err != nil {
			dev.Close()
			continue
		}
		dev.Close()

		if strings.Contains(name, deviceName) {
			logInfo("タッチパッド発見: %s (%s)", name, f)
			return f, nil
		}
	}
	return "", nil
}

func main() {
	// Configure logging
	log.SetFlags(0) // Raw messages without standard timestamps (mimics Python's format)
	log.SetOutput(os.Stderr)

	var configPath string
	flag.StringVar(&configPath, "config", "", "設定ファイルのパス (デフォルト: /etc/wheelpad/config.toml)")
	flag.StringVar(&configPath, "c", "", "設定ファイルのパス (ショートカット)")
	flag.BoolVar(&debugEnabled, "debug", false, "デバッグログを有効にする")
	flag.Parse()

	// Handle standard log formats with time if not custom
	log.SetFlags(log.Ltime)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		logError("設定ファイルの読み込みに失敗しました: %v", err)
		os.Exit(1)
	}

	devicePath, err := findTouchpad(cfg.Device.Name)
	if err != nil {
		logError("タッチパッドの走査中にエラーが発生しました: %v", err)
		os.Exit(1)
	}
	if devicePath == "" {
		logError("タッチパッドが見つかりません: %s", cfg.Device.Name)
		os.Exit(1)
	}

	daemon, err := NewWheelpadDaemon(devicePath, cfg)
	if err != nil {
		logError("デーモンの初期化に失敗しました (パーミッションを確認してください): %v", err)
		os.Exit(1)
	}

	// Capture OS signals for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logInfo("シグナルを受け取りました: %v。シャットダウンします...", sig)
		daemon.Cleanup()
		os.Exit(0)
	}()

	err = daemon.Run()
	if err != nil {
		logError("デーモンの動作中にエラーが発生しました: %v", err)
		daemon.Cleanup()
		os.Exit(1)
	}
}

func init() {
	// Custom flag usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  -c, --config PATH  設定ファイルのパス (デフォルト: /etc/wheelpad/config.toml)\n")
		fmt.Fprintf(os.Stderr, "      --debug        デバッグログを有効にする\n")
	}
}
