package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultReconnectDelay = 5 * time.Second

type fileConfig struct {
	WSURL                 string   `json:"wsUrl"`
	SeriesPaths           []string `json:"seriesPaths"`
	Host                  HostInfo `json:"host"`
	ReconnectDelaySeconds float64  `json:"reconnectDelaySeconds"`
}

type HostConfig struct {
	Path           string
	WSURL          string
	SeriesPaths    []string
	Host           HostInfo
	ReconnectDelay time.Duration
}

func loadConfig(configPath string) (HostConfig, error) {
	resolvedPath, err := filepath.Abs(configPath)
	if err != nil {
		return HostConfig{}, fmt.Errorf("resolve config path: %w", err)
	}

	configBytes, err := os.ReadFile(resolvedPath)
	if err != nil {
		return HostConfig{}, fmt.Errorf("read config file: %w", err)
	}

	var raw fileConfig
	if err := json.Unmarshal(configBytes, &raw); err != nil {
		return HostConfig{}, fmt.Errorf("decode config file: %w", err)
	}

	wsURL := strings.TrimSpace(raw.WSURL)
	if wsURL == "" {
		return HostConfig{}, fmt.Errorf("wsUrl must not be empty")
	}
	if len(raw.SeriesPaths) == 0 {
		return HostConfig{}, fmt.Errorf("seriesPaths must contain at least one path")
	}
	if strings.TrimSpace(raw.Host.ID) == "" {
		return HostConfig{}, fmt.Errorf("host.id must not be empty")
	}
	if strings.TrimSpace(raw.Host.Username) == "" {
		return HostConfig{}, fmt.Errorf("host.username must not be empty")
	}

	seriesPaths := make([]string, 0, len(raw.SeriesPaths))
	for _, seriesPath := range raw.SeriesPaths {
		trimmed := strings.TrimSpace(seriesPath)
		if trimmed == "" {
			return HostConfig{}, fmt.Errorf("seriesPaths must not contain empty paths")
		}
		resolvedSeriesPath, err := filepath.Abs(trimmed)
		if err != nil {
			return HostConfig{}, fmt.Errorf("resolve series path %q: %w", trimmed, err)
		}
		seriesPaths = append(seriesPaths, resolvedSeriesPath)
	}

	reconnectDelay := defaultReconnectDelay
	if raw.ReconnectDelaySeconds != 0 {
		if raw.ReconnectDelaySeconds <= 0 {
			return HostConfig{}, fmt.Errorf("reconnectDelaySeconds must be greater than 0")
		}
		reconnectDelay = time.Duration(raw.ReconnectDelaySeconds * float64(time.Second))
	}

	return HostConfig{
		Path:           resolvedPath,
		WSURL:          wsURL,
		SeriesPaths:    seriesPaths,
		Host:           raw.Host,
		ReconnectDelay: reconnectDelay,
	}, nil
}
