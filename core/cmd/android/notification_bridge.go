//go:build android || jni

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/om051p/phonebridge/core/pkg/notification"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

// NotificationSender abstracts sending notification wire bytes (MediaTransport implements it).
type NotificationSender interface {
	SendNotification(wireBytes []byte) error
}

// NotificationBridge bridges Android notification events to the WebRTC "notifications" DataChannel (DEC-028).
type NotificationBridge struct {
	mu           sync.Mutex
	initialized  atomic.Bool
	channelOpen  atomic.Bool
	sender       NotificationSender
	postedCount  atomic.Uint64
	removedCount atomic.Uint64
	droppedCount atomic.Uint64
	sendErrors   atomic.Uint64
}

var globalNotification atomic.Pointer[NotificationBridge]

func currentNotificationBridge() *NotificationBridge {
	if b := globalNotification.Load(); b != nil {
		return b
	}
	b := &NotificationBridge{}
	globalNotification.Store(b)
	return b
}

func (b *NotificationBridge) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.initialized.Store(true)
	return nil
}

func (b *NotificationBridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.initialized.Store(false)
	b.channelOpen.Store(false)
}

func (b *NotificationBridge) SetSender(s NotificationSender) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sender = s
}

func (b *NotificationBridge) getSender() NotificationSender {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sender != nil {
		return b.sender
	}
	if tr := currentTransport(); tr != nil {
		return tr
	}
	return nil
}

func (b *NotificationBridge) OnChannelOpen() {
	b.channelOpen.Store(true)
}

func (b *NotificationBridge) OnChannelClose() {
	b.channelOpen.Store(false)
}

// PostNotification constructs, validates, and dispatches a NotificationPosted event over WebRTC.
// Under zero-logging policy, title/text contents are never logged.
func (b *NotificationBridge) PostNotification(
	key, packageName, appName, title, text, subText string,
	postTimeMs int64,
	isOngoing, isClearable bool,
	category string,
) error {
	if !b.initialized.Load() {
		return errors.New("notification: bridge not initialized")
	}
	if !b.channelOpen.Load() {
		b.droppedCount.Add(1)
		return nil
	}

	cleanKey, err := notification.SanitizeString(key)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}
	cleanPackage, err := notification.SanitizeString(packageName)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}
	cleanApp, err := notification.SanitizeString(appName)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}
	cleanTitle, err := notification.SanitizeString(title)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}
	cleanText, err := notification.SanitizeString(text)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}
	cleanSubText, err := notification.SanitizeString(subText)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}
	cleanCategory, err := notification.SanitizeString(category)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}

	posted := &phonebridgev1.NotificationPosted{
		Key:         cleanKey,
		PackageName: cleanPackage,
		AppName:     cleanApp,
		Title:       cleanTitle,
		Text:        cleanText,
		SubText:     cleanSubText,
		PostTimeMs:  postTimeMs,
		IsOngoing:   isOngoing,
		IsClearable: isClearable,
		Category:    cleanCategory,
	}

	if err := notification.ValidateNotificationPosted(posted); err != nil {
		b.droppedCount.Add(1)
		return err
	}

	frame := &phonebridgev1.NotificationFrame{
		Version:     1,
		TimestampMs: uint64(time.Now().UnixMilli()),
		Event: &phonebridgev1.NotificationFrame_Posted{
			Posted: posted,
		},
	}

	data, err := proto.Marshal(frame)
	if err != nil {
		b.droppedCount.Add(1)
		return fmt.Errorf("notification: marshal frame: %w", err)
	}

	snd := b.getSender()
	if snd == nil {
		b.droppedCount.Add(1)
		return errors.New("notification: no active transport")
	}

	if err := snd.SendNotification(data); err != nil {
		b.sendErrors.Add(1)
		return err
	}
	b.postedCount.Add(1)
	return nil
}

// RemoveNotification constructs, validates, and dispatches a NotificationRemoved event over WebRTC.
func (b *NotificationBridge) RemoveNotification(key, packageName string, reason int32) error {
	if !b.initialized.Load() {
		return errors.New("notification: bridge not initialized")
	}
	if !b.channelOpen.Load() {
		b.droppedCount.Add(1)
		return nil
	}

	cleanKey, err := notification.SanitizeString(key)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}
	cleanPackage, err := notification.SanitizeString(packageName)
	if err != nil {
		b.droppedCount.Add(1)
		return err
	}

	removed := &phonebridgev1.NotificationRemoved{
		Key:         cleanKey,
		PackageName: cleanPackage,
		Reason:      reason,
	}

	if err := notification.ValidateNotificationRemoved(removed); err != nil {
		b.droppedCount.Add(1)
		return err
	}

	frame := &phonebridgev1.NotificationFrame{
		Version:     1,
		TimestampMs: uint64(time.Now().UnixMilli()),
		Event: &phonebridgev1.NotificationFrame_Removed{
			Removed: removed,
		},
	}

	data, err := proto.Marshal(frame)
	if err != nil {
		b.droppedCount.Add(1)
		return fmt.Errorf("notification: marshal frame: %w", err)
	}

	snd := b.getSender()
	if snd == nil {
		b.droppedCount.Add(1)
		return errors.New("notification: no active transport")
	}

	if err := snd.SendNotification(data); err != nil {
		b.sendErrors.Add(1)
		return err
	}
	b.removedCount.Add(1)
	return nil
}

// NotificationStatsJSON serializes notification bridge statistics for debugging.
func (b *NotificationBridge) NotificationStatsJSON() []byte {
	stats := map[string]any{
		"initialized":  b.initialized.Load(),
		"channel_open": b.channelOpen.Load(),
		"posted":       b.postedCount.Load(),
		"removed":      b.removedCount.Load(),
		"dropped":      b.droppedCount.Load(),
		"send_errors":  b.sendErrors.Load(),
	}
	data, _ := json.Marshal(stats)
	return data
}
