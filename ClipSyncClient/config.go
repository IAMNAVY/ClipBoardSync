package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SyncMode controls the clipboard sync direction.
const (
	SyncBidirectional = "bidirectional" // phone ↔ cloud
	SyncUploadOnly    = "upload_only"   // phone → cloud
	SyncDownloadOnly  = "download_only" // cloud → phone
	SyncOff           = "off"           // disabled
)

// AppConfig holds user-persistent configuration.
type AppConfig struct {
	ServerURL  string `json:"server_url"`
	Username   string `json:"username"`
	Token      string `json:"token"`
	AutoStart  bool   `json:"auto_start"`
	DeviceName string `json:"device_name"`
	SyncMode   string `json:"sync_mode"`
}

// GetSyncMode returns the sync mode, defaulting to bidirectional.
func (c *AppConfig) GetSyncMode() string {
	switch c.SyncMode {
	case SyncBidirectional, SyncUploadOnly, SyncDownloadOnly, SyncOff:
		return c.SyncMode
	default:
		return SyncBidirectional
	}
}

// ShouldUpload returns true if local clipboard changes should be uploaded.
func (c *AppConfig) ShouldUpload() bool {
	m := c.GetSyncMode()
	return m == SyncBidirectional || m == SyncUploadOnly
}

// ShouldDownload returns true if remote clipboard changes should be written locally.
func (c *AppConfig) ShouldDownload() bool {
	m := c.GetSyncMode()
	return m == SyncBidirectional || m == SyncDownloadOnly
}

// GetDeviceName returns the configured device name, falling back to hostname.
func GetDeviceName(cfg *AppConfig) string {
	if cfg != nil && cfg.DeviceName != "" {
		return cfg.DeviceName
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "Desktop Client"
	}
	return hostname
}

// configDir returns %APPDATA%\ClipSyncClient, creating it if necessary.
func configDir() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", fmt.Errorf("APPDATA environment variable not set")
	}
	dir := filepath.Join(appData, "ClipSyncClient")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// configPath returns the full path to the config file.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadConfig loads the config from disk, returning a zero-value config if not found.
func LoadConfig() (*AppConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AppConfig{}, nil
		}
		return nil, err
	}

	// Decode base64 obfuscation layer
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("config decode error: %w", err)
	}

	var cfg AppConfig
	if err := json.Unmarshal(decoded, &cfg); err != nil {
		return nil, fmt.Errorf("config unmarshal error: %w", err)
	}
	return &cfg, nil
}

// SaveConfig persists the config to disk with base64 obfuscation.
func SaveConfig(cfg *AppConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return os.WriteFile(path, []byte(encoded), 0600)
}
