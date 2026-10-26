package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	Black = "black"
	White = "white"
)

type Config struct {
	Background string `json:"background"`
	Paused     bool   `json:"paused"`
}

func Default() Config {
	return Config{Background: Black}
}

func (c Config) White() bool {
	return c.Background == White
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "towascii", "config.json"), nil
}

// Load returns the defaults when the file is missing. A corrupt file also
// yields the defaults, together with an error the caller may log.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	c := Default()
	if err := json.Unmarshal(data, &c); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Background != White {
		c.Background = Black
	}
	return c, nil
}

// Save writes atomically so a crash mid-write never leaves a truncated file.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
