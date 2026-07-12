package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type DeviceConfig struct {
	Name string `toml:"name"`
}

type WheelpadConfig struct {
	CenterX       int     `toml:"center_x"`
	CenterY       int     `toml:"center_y"`
	Deadzone      float64 `toml:"deadzone"`
	Sensitivity   float64 `toml:"sensitivity"`
	HiresStep     int32   `toml:"hires_step"`
	NaturalScroll bool    `toml:"natural_scroll"`
}

type Threshold struct {
	Velocity   float64 `toml:"velocity"`
	Multiplier int32   `toml:"multiplier"`
}

type SpeedConfig struct {
	Thresholds []Threshold `toml:"thresholds"`
}

type InertiaConfig struct {
	Enabled     bool    `toml:"enabled"`
	Friction    float64 `toml:"friction"`
	MinVelocity float64 `toml:"min_velocity"`
	Interval    float64 `toml:"interval"` // in seconds
}

type Config struct {
	Device   DeviceConfig   `toml:"device"`
	Wheelpad WheelpadConfig `toml:"wheelpad"`
	Speed    SpeedConfig    `toml:"speed"`
	Inertia  InertiaConfig  `toml:"inertia"`
}

// NewDefaultConfig returns a configuration pre-populated with standard default values.
func NewDefaultConfig() Config {
	return Config{
		Device: DeviceConfig{
			Name: "Synaptics TM3562-003",
		},
		Wheelpad: WheelpadConfig{
			CenterX:       264,
			CenterY:       264,
			Deadzone:      195,
			Sensitivity:   0.3,
			HiresStep:     60,
			NaturalScroll: false,
		},
		Speed: SpeedConfig{
			Thresholds: []Threshold{
				{Velocity: 8.0, Multiplier: 4},
				{Velocity: 4.0, Multiplier: 3},
				{Velocity: 2.0, Multiplier: 2},
			},
		},
		Inertia: InertiaConfig{
			Enabled:     true,
			Friction:    0.85,
			MinVelocity: 0.5,
			Interval:    0.016,
		},
	}
}

// expandPath resolves paths containing tilde (~) to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// LoadConfig loads the configuration from a path or the default fallback locations,
// merging it with default values.
func LoadConfig(configPath string) (Config, error) {
	cfg := NewDefaultConfig()

	if configPath != "" {
		path := expandPath(configPath)
		if _, err := os.Stat(path); err == nil {
			logInfo("設定ファイル読み込み: %s", path)
			data, err := os.ReadFile(path)
			if err != nil {
				return cfg, err
			}
			err = toml.Unmarshal(data, &cfg)
			return cfg, err
		}
		logWarn("指定された設定ファイルが見つかりません: %s (デフォルト値を使用)", path)
		return cfg, nil
	}

	searchPaths := []string{
		"/etc/wheelpad/config.toml",
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			logInfo("設定ファイル読み込み: %s", path)
			data, err := os.ReadFile(path)
			if err != nil {
				return cfg, err
			}
			err = toml.Unmarshal(data, &cfg)
			return cfg, err
		}
	}

	logInfo("設定ファイルなし — デフォルト値を使用")
	return cfg, nil
}
