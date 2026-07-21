package iface

import (
	"context"
	"time"
)

// =============================================================================
// 事件系统
// =============================================================================

// EventType 是事件类型的标识符。
type EventType string

// Event 是 Agent 事件的轻量级表示。
type Event interface {
	Type() EventType
	Payload() any
}

// EventHandler 是事件处理回调。
type EventHandler func(Event)

// EventBus 提供 agent 生命周期事件的发布/订阅。
type EventBus interface {
	On(eventType EventType, handler EventHandler) func()
	OnAll(handler EventHandler) func()
	Emit(event Event)
	EmitMustDeliver(ctx context.Context, event Event)
	Close()
}

// SimpleEvent 是最简事件实现。
type SimpleEvent struct {
	typ     EventType
	at      time.Time
	payload any
}

func NewEvent(typ EventType, payload any) SimpleEvent {
	return SimpleEvent{typ: typ, at: time.Now(), payload: payload}
}

func (e SimpleEvent) Type() EventType { return e.typ }
func (e SimpleEvent) Time() time.Time { return e.at }
func (e SimpleEvent) Payload() any    { return e.payload }
