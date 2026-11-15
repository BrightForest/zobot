// Package config reads process environment for the bot.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is everything the process needs to start. Secrets stay in the fields
// and are not logged.
type Config struct {
	BotToken    string
	DvachURL    string
	CheckEvery  time.Duration
	PostgresDSN string
}

// FromEnv loads configuration. CHECK_RATE_SECONDS defaults to 60 when empty
// or not a positive integer. Every other variable is required.
func FromEnv() (Config, error) {
	var missing []string
	get := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	token := get("BOT_TOKEN")
	dvachURL := get("2CH_URL")
	user := get("POSTGRES_USER")
	password := get("POSTGRES_PASSWORD")
	host := get("POSTGRES_HOST")
	port := get("POSTGRES_PORT")
	database := get("POSTGRES_DB")
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing env: %s", strings.Join(missing, ", "))
	}

	dsn, err := postgresDSN(user, password, host, port, database)
	if err != nil {
		return Config{}, err
	}
	return Config{
		BotToken:    token,
		DvachURL:    dvachURL,
		CheckEvery:  checkEvery(os.Getenv("CHECK_RATE_SECONDS")),
		PostgresDSN: dsn,
	}, nil
}

func checkEvery(raw string) time.Duration {
	const fallback = 60
	if raw == "" {
		return fallback * time.Second
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback * time.Second
	}
	return time.Duration(n) * time.Second
}

func postgresDSN(user, password, host, port, database string) (string, error) {
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("POSTGRES_PORT: %w", err)
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     net.JoinHostPort(host, port),
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
	}
	return u.String(), nil
}
