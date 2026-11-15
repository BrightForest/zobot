// Package store is the Postgres persistence for subscribers and regexes.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Subscriber is one Telegram chat row.
type Subscriber struct {
	ChatID   int64
	Username string
	IsActive bool
	Blocked  bool
}

// Postgres is a small SQL wrapper. It is safe for concurrent use.
type Postgres struct {
	db *sql.DB
}

// Open pings the database before returning.
func Open(ctx context.Context, dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Postgres{db: db}, nil
}

// Close releases the pool.
func (p *Postgres) Close() error {
	if p == nil || p.db == nil {
		return nil
	}
	return p.db.Close()
}

// Migrate adds the blocked flag used to remember users who banned the bot.
func (p *Postgres) Migrate(ctx context.Context) error {
	const q = `
ALTER TABLE public.subscribers
    ADD COLUMN IF NOT EXISTS blocked boolean NOT NULL DEFAULT false`
	if _, err := p.db.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("migrate blocked: %w", err)
	}
	return nil
}

// ListSubscribers returns every row.
func (p *Postgres) ListSubscribers(ctx context.Context) ([]Subscriber, error) {
	const q = `SELECT chatid, username, isactive, blocked FROM public.subscribers`
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list subscribers: %w", err)
	}
	defer rows.Close()

	var out []Subscriber
	for rows.Next() {
		var s Subscriber
		if err := rows.Scan(&s.ChatID, &s.Username, &s.IsActive, &s.Blocked); err != nil {
			return nil, fmt.Errorf("scan subscriber: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list subscribers: %w", err)
	}
	return out, nil
}

// AddSubscriber inserts a chat. A duplicate chat id is ignored.
func (p *Postgres) AddSubscriber(ctx context.Context, s Subscriber) error {
	const q = `
INSERT INTO public.subscribers (chatid, username, isactive, blocked)
VALUES ($1, $2, $3, $4)
ON CONFLICT (chatid) DO NOTHING`
	if _, err := p.db.ExecContext(ctx, q, s.ChatID, s.Username, s.IsActive, s.Blocked); err != nil {
		return fmt.Errorf("add subscriber: %w", err)
	}
	return nil
}

// SetActive toggles /pause and does not touch blocked.
func (p *Postgres) SetActive(ctx context.Context, chatID int64, active bool) error {
	const q = `UPDATE public.subscribers SET isactive = $2 WHERE chatid = $1`
	if _, err := p.db.ExecContext(ctx, q, chatID, active); err != nil {
		return fmt.Errorf("set active: %w", err)
	}
	return nil
}

// Activate is /disco: delivery on, blocked flag cleared so a user who
// unblocked the bot can receive posts again.
func (p *Postgres) Activate(ctx context.Context, chatID int64) error {
	const q = `UPDATE public.subscribers SET isactive = true, blocked = false WHERE chatid = $1`
	if _, err := p.db.ExecContext(ctx, q, chatID); err != nil {
		return fmt.Errorf("activate: %w", err)
	}
	return nil
}

// SetBlocked persists a Telegram Forbidden result.
func (p *Postgres) SetBlocked(ctx context.Context, chatID int64, blocked bool) error {
	const q = `UPDATE public.subscribers SET blocked = $2 WHERE chatid = $1`
	if _, err := p.db.ExecContext(ctx, q, chatID, blocked); err != nil {
		return fmt.Errorf("set blocked: %w", err)
	}
	return nil
}

// Regexes returns raw patterns from the table. Callers compile them.
func (p *Postgres) Regexes(ctx context.Context) ([]string, error) {
	const q = `SELECT reg FROM public.regexes`
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list regexes: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var reg string
		if err := rows.Scan(&reg); err != nil {
			return nil, fmt.Errorf("scan regex: %w", err)
		}
		out = append(out, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list regexes: %w", err)
	}
	return out, nil
}
