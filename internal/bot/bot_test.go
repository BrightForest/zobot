package bot

import (
	"errors"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/BrightForest/zobot/internal/outbound"
)

func TestClassifyRateLimitAndForbidden(t *testing.T) {
	rl := classify(&tgbotapi.Error{
		Code:               429,
		Message:            "Too Many Requests: retry after 5",
		ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 5},
	})
	var rate *outbound.RateLimitError
	if !errors.As(rl, &rate) || rate.RetryAfter != 5*time.Second {
		t.Fatalf("rate: %#v", rl)
	}

	fb := classify(&tgbotapi.Error{Code: 403, Message: "Forbidden: bot was blocked by the user"})
	var forbidden *outbound.ForbiddenError
	if !errors.As(fb, &forbidden) {
		t.Fatalf("forbidden: %#v", fb)
	}

	other := classify(&tgbotapi.Error{Code: 400, Message: "Bad Request"})
	if errors.As(other, &rate) || errors.As(other, &forbidden) {
		t.Fatalf("other should stay plain: %v", other)
	}
}

func TestRedactToken(t *testing.T) {
	err := redactToken("secret-token", errors.New(`Post "https://api.telegram.org/botsecret-token/sendMessage": timeout`))
	if err == nil || strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "<token>") {
		t.Fatalf("redact: %v", err)
	}
	apiErr := &tgbotapi.Error{Code: 400, Message: "Bad Request"}
	if got := redactToken("secret-token", apiErr); !errors.Is(got, apiErr) && got.Error() != apiErr.Error() {
		t.Fatalf("api error should stay typed, got %T %v", got, got)
	}
}
