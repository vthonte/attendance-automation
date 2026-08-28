package core

import (
	"fmt"
	"sync"
	"time"
)

type StatusEvent struct {
	Status      string    `json:"status"`
	DateKey     string    `json:"date_key"`
	DisplayText string    `json:"display_text"`
	ColorName   string    `json:"color_name"`
	BarVisible  bool      `json:"bar_visible"`
	Timestamp   time.Time `json:"timestamp"`
}

func FormatDisplayDate(dateKey string) string {
	if dateKey == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", dateKey)
	if err != nil {
		return dateKey
	}
	return t.Format("Jan 2")
}

func GetStatusDisplay(status, dateKey string, showLoggedDate bool) (text string, colorName string) {
	switch status {
	case "in":
		colorName = "lightgreen"
		if showLoggedDate && dateKey != "" {
			text = fmt.Sprintf("Logged %s", FormatDisplayDate(dateKey))
		} else {
			text = "Logged"
		}
	case "run":
		colorName = "khaki"
		text = "Checking"
	case "error":
		colorName = "lightcoral"
		text = "Needs attention"
	case "out":
		colorName = "lightgray"
		text = "Not logged"
	default:
		colorName = "lightgray"
		text = status
	}
	return text, colorName
}

type EventBus struct {
	mu          sync.RWMutex
	subscribers []chan StatusEvent
}

func NewEventBus() *EventBus {
	return &EventBus{}
}

func (b *EventBus) Subscribe() chan StatusEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan StatusEvent, 16)
	b.subscribers = append(b.subscribers, ch)
	return ch
}

func (b *EventBus) Publish(ev StatusEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
			// Drain if full to prevent blocking
			select {
			case <-ch:
			default:
			}
			ch <- ev
		}
	}
}
