package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// loginResponse maps the server's /api/login JSON response.
type loginResponse struct {
	Message      string `json:"message"`
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	UserID       uint   `json:"user_id"`
	Username     string `json:"username"`
	Error        string `json:"error"`
}

// Login authenticates with the server and returns the access token and refresh token.
func Login(serverURL, username, password string) (accessToken, refreshToken string, err error) {
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})

	resp, err := httpClient.Post(
		serverURL+"/api/login",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", "", fmt.Errorf("连接服务器失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var lr loginResponse
	if err := json.Unmarshal(respBody, &lr); err != nil {
		return "", "", fmt.Errorf("解析响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := lr.Error
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return "", "", fmt.Errorf("登录失败: %s", msg)
	}

	if lr.Token == "" {
		return "", "", fmt.Errorf("服务器未返回 Token")
	}
	return lr.Token, lr.RefreshToken, nil
}

// RefreshAccessToken exchanges a refresh token for a new access+refresh token pair.
func RefreshAccessToken(serverURL, refreshToken string) (newAccessToken, newRefreshToken string, err error) {
	body, _ := json.Marshal(map[string]string{
		"refresh_token": refreshToken,
	})

	resp, err := httpClient.Post(
		serverURL+"/api/refresh",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", "", fmt.Errorf("连接服务器失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var lr loginResponse
	if err := json.Unmarshal(respBody, &lr); err != nil {
		return "", "", fmt.Errorf("解析响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := lr.Error
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return "", "", fmt.Errorf("刷新 Token 失败: %s", msg)
	}

	if lr.Token == "" {
		return "", "", fmt.Errorf("服务器未返回 Token")
	}
	return lr.Token, lr.RefreshToken, nil
}

// PushClipboard sends clipboard content to the server.
func PushClipboard(serverURL, token, content, deviceName string) error {
	body, _ := json.Marshal(map[string]string{
		"content":     content,
		"device_name": deviceName,
	})

	req, err := http.NewRequest("POST", serverURL+"/api/clipboard", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传剪贴板失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("上传失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}
