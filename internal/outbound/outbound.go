// Package outbound is the send port between the engine and Telegram.
package outbound

import (
	"context"
	"time"
)

// Sender delivers a text message to one chat.
type Sender interface {
	Send(ctx context.Context, chatID int64, text string) error
}

// ForbiddenError means the user blocked the bot or the chat is gone.
type ForbiddenError struct {
	Err error
}

func (e *ForbiddenError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "forbidden"
}

func (e *ForbiddenError) Unwrap() error { return e.Err }

// RateLimitError is Telegram 429. RetryAfter is how long to wait.
type RateLimitError struct {
	RetryAfter time.Duration
	Err        error
}

func (e *RateLimitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "rate limited"
}

func (e *RateLimitError) Unwrap() error { return e.Err }
