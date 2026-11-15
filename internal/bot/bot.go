// Package bot talks to the Telegram Bot API.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/BrightForest/zobot/internal/outbound"
)

// Handler turns a slash-command into a reply. An empty reply is not sent.
type Handler interface {
	HandleCommand(ctx context.Context, chatID int64, username, command string) string
}

// Client is an authorized bot. Connect fails fast when the token is rejected.
type Client struct {
	api    *tgbotapi.BotAPI
	token  string
	stopMu sync.Once
}

// Connect calls getMe. A bad token returns an error and the process should exit.
func Connect(token string) (*Client, error) {
	httpClient := &http.Client{Timeout: 75 * time.Second}
	api, err := tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		return nil, fmt.Errorf("telegram auth: %w", redact(token, err))
	}
	return &Client{api: api, token: token}, nil
}

// Username is the bot's @name without the at-sign.
func (c *Client) Username() string {
	if c == nil || c.api == nil {
		return ""
	}
	return c.api.Self.UserName
}

// Send implements outbound.Sender. 429 and Forbidden are typed errors.
func (c *Client) Send(ctx context.Context, chatID int64, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := c.api.Send(msg)
	if err != nil {
		return classify(redactToken(c.token, err))
	}
	return nil
}

// Run reads commands until ctx is cancelled. Shutdown closes the long-poll.
func (c *Client) Run(ctx context.Context, handler Handler) error {
	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = 60
	updates := c.api.GetUpdatesChan(cfg)

	go func() {
		<-ctx.Done()
		c.stop()
	}()

	for update := range updates {
		if ctx.Err() != nil {
			return nil
		}
		c.handle(ctx, handler, update)
	}
	return nil
}

func (c *Client) stop() {
	c.stopMu.Do(func() {
		c.api.StopReceivingUpdates()
	})
}

func (c *Client) handle(ctx context.Context, handler Handler, update tgbotapi.Update) {
	if update.Message == nil || !update.Message.IsCommand() {
		return
	}
	username := ""
	if update.Message.From != nil {
		username = update.Message.From.UserName
	}
	if username == "" {
		username = update.Message.Chat.UserName
	}
	text := handler.HandleCommand(ctx, update.Message.Chat.ID, username, update.Message.Command())
	if text == "" {
		return
	}
	if _, err := c.api.Send(tgbotapi.NewMessage(update.Message.Chat.ID, text)); err != nil {
		log.Printf("reply to %d: %v", update.Message.Chat.ID, redact(c.token, err))
	}
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *tgbotapi.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code == 429 {
			wait := time.Duration(apiErr.RetryAfter) * time.Second
			if wait <= 0 {
				wait = 3 * time.Second
			}
			return &outbound.RateLimitError{RetryAfter: wait, Err: err}
		}
		if apiErr.Code == 403 || strings.Contains(apiErr.Message, "Forbidden") {
			return &outbound.ForbiddenError{Err: err}
		}
		return err
	}
	msg := err.Error()
	if strings.Contains(msg, "Forbidden") {
		return &outbound.ForbiddenError{Err: err}
	}
	if strings.Contains(strings.ToLower(msg), "too many requests") || strings.Contains(strings.ToLower(msg), "retry after") {
		return &outbound.RateLimitError{RetryAfter: 3 * time.Second, Err: err}
	}
	return err
}

func redactToken(token string, err error) error {
	if err == nil || token == "" {
		return err
	}
	var apiErr *tgbotapi.Error
	if errors.As(err, &apiErr) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), token, "<token>"))
}

func redact(token string, err error) error {
	if err == nil || token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), token, "<token>"))
}
