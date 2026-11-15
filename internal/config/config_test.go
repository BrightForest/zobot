package config

import (
	"strings"
	"testing"
	"time"
)

func TestFromEnvMissing(t *testing.T) {
	t.Setenv("BOT_TOKEN", "")
	t.Setenv("2CH_URL", "")
	t.Setenv("POSTGRES_USER", "")
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv("POSTGRES_HOST", "")
	t.Setenv("POSTGRES_PORT", "")
	t.Setenv("POSTGRES_DB", "")
	_, err := FromEnv()
	if err == nil || !strings.Contains(err.Error(), "BOT_TOKEN") {
		t.Fatalf("expected missing env, got %v", err)
	}
}

func TestFromEnvDSNEscapesPassword(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("2CH_URL", "https://2ch.su")
	t.Setenv("POSTGRES_USER", "zobot")
	t.Setenv("POSTGRES_PASSWORD", "p@ss/word")
	t.Setenv("POSTGRES_HOST", "postgres")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DB", "zobot")
	t.Setenv("CHECK_RATE_SECONDS", "nope")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CheckEvery != 60*time.Second {
		t.Fatalf("fallback rate = %s", cfg.CheckEvery)
	}
	if !strings.Contains(cfg.PostgresDSN, "p%40ss%2Fword") {
		t.Fatalf("password not escaped: %s", cfg.PostgresDSN)
	}
	if !strings.Contains(cfg.PostgresDSN, "@postgres:5432/zobot?sslmode=disable") {
		t.Fatalf("dsn shape: %s", cfg.PostgresDSN)
	}
}

func TestCheckEvery(t *testing.T) {
	if checkEvery("15") != 15*time.Second {
		t.Fatal("positive integer")
	}
	if checkEvery("-1") != 60*time.Second || checkEvery("0") != 60*time.Second {
		t.Fatal("non-positive should fall back")
	}
}
