package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	appName     = "ClipSyncClient"
	registryKey = `Software\Microsoft\Windows\CurrentVersion\Run`
)

// IsAutoStartEnabled checks if the auto-start registry entry exists.
func IsAutoStartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(appName)
	return err == nil
}

// EnableAutoStart adds the current exe to the auto-start registry.
func EnableAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	exePath, _ = filepath.Abs(exePath)

	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(appName, `"`+exePath+`"`); err != nil {
		return fmt.Errorf("写入注册表失败: %w", err)
	}

	log.Println("[autostart] 已启用开机自启动")
	return nil
}

// DisableAutoStart removes the auto-start registry entry.
func DisableAutoStart() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	if err := key.DeleteValue(appName); err != nil {
		return fmt.Errorf("删除注册表项失败: %w", err)
	}

	log.Println("[autostart] 已禁用开机自启动")
	return nil
}
