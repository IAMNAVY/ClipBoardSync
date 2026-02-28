package main

import (
	"log"
	"sync"
	"time"

	"github.com/atotto/clipboard"
)

// ClipMonitor polls the system clipboard and detects changes.
// It implements an anti-loop mechanism to avoid re-uploading content
// that was just written by the WebSocket client.
type ClipMonitor struct {
	onNewClip func(content string) // called when a user-initiated copy is detected

	mu               sync.Mutex
	lastContent      string    // last known clipboard content
	lastRemoteWrite  string    // content that was written by the WS client
	skipNextChange   bool      // flag to skip the very next change detection
	lastWriteTime    time.Time // time of last WriteClipboard call (anti-loop enhancement)

	// Offline queue: stores clipboard content when WS is disconnected
	offlineQueue []string

	stopCh chan struct{}
	done   chan struct{}
}

// NewClipMonitor creates a new clipboard monitor.
func NewClipMonitor(onNewClip func(string)) *ClipMonitor {
	return &ClipMonitor{
		onNewClip: onNewClip,
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}
}

// Start begins polling the clipboard in a goroutine.
func (m *ClipMonitor) Start() {
	// Read initial clipboard content so we don't upload pre-existing content
	if content, err := clipboard.ReadAll(); err == nil {
		m.lastContent = content
	}
	go m.pollLoop()
}

// Stop ends the polling loop.
func (m *ClipMonitor) Stop() {
	close(m.stopCh)
	<-m.done
}

// WriteClipboard writes content from a remote source to the local clipboard.
// It sets the anti-loop flag so the monitor will not re-upload this content.
func (m *ClipMonitor) WriteClipboard(content string) {
	m.mu.Lock()
	m.lastRemoteWrite = content
	m.skipNextChange = true
	m.lastContent = content
	m.lastWriteTime = time.Now()
	m.mu.Unlock()

	if err := clipboard.WriteAll(content); err != nil {
		log.Printf("[clip] 写入剪贴板失败: %v", err)
	} else {
		log.Printf("[clip] 已将远程内容写入剪贴板 (%d 字符)", len(content))
	}
}

// Enqueue adds clipboard content to the offline queue for later sending.
func (m *ClipMonitor) Enqueue(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offlineQueue = append(m.offlineQueue, content)
	log.Printf("[clip] 已暂存到离线队列 (队列长度: %d)", len(m.offlineQueue))
}

// FlushQueue sends all queued clipboard content using the provided push function.
// Called when network connection is restored.
func (m *ClipMonitor) FlushQueue(pushFn func(string) error) {
	m.mu.Lock()
	queue := make([]string, len(m.offlineQueue))
	copy(queue, m.offlineQueue)
	m.offlineQueue = nil
	m.mu.Unlock()

	if len(queue) == 0 {
		return
	}

	log.Printf("[clip] 开始补发离线队列 (%d 条)", len(queue))
	for i, content := range queue {
		if err := pushFn(content); err != nil {
			log.Printf("[clip] 补发第 %d 条失败: %v", i+1, err)
			// Re-enqueue remaining items
			m.mu.Lock()
			m.offlineQueue = append(queue[i:], m.offlineQueue...)
			m.mu.Unlock()
			return
		}
		log.Printf("[clip] 补发第 %d/%d 条成功", i+1, len(queue))
	}
	log.Println("[clip] 离线队列补发完成")
}

func (m *ClipMonitor) pollLoop() {
	defer close(m.done)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkClipboard()
		}
	}
}

func (m *ClipMonitor) checkClipboard() {
	content, err := clipboard.ReadAll()
	if err != nil {
		return // clipboard may be locked by another app, just skip
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// No change
	if content == m.lastContent {
		return
	}

	m.lastContent = content

	// Anti-loop: if this content was just written by the remote, skip it
	if m.skipNextChange {
		m.skipNextChange = false
		if content == m.lastRemoteWrite {
			log.Println("[clip] 跳过远程写入内容，防止死循环")
			return
		}
	}

	// Enhanced anti-loop: if we just wrote to clipboard recently (within 2s)
	// and the content matches the last remote write, skip it.
	// This handles delayed clipboard re-writes from paste events in rich text editors.
	if !m.lastWriteTime.IsZero() && time.Since(m.lastWriteTime) < 2*time.Second {
		if content == m.lastRemoteWrite {
			log.Println("[clip] 检测到粘贴引起的剪贴板重写，忽略 (距上次远程写入 <2s)")
			return
		}
	}

	// Empty content, skip
	if content == "" {
		return
	}

	log.Printf("[clip] 检测到剪贴板变更 (%d 字符)，准备上传", len(content))
	if m.onNewClip != nil {
		// Call asynchronously to avoid blocking the poll loop
		go m.onNewClip(content)
	}
}
