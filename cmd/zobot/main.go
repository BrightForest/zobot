package main

import (
	"context"
	"log"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/BrightForest/zobot/internal/bot"
	"github.com/BrightForest/zobot/internal/config"
	"github.com/BrightForest/zobot/internal/dvach"
	"github.com/BrightForest/zobot/internal/engine"
	"github.com/BrightForest/zobot/internal/store"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg config.Config) error {
	db, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	client, err := bot.Connect(cfg.BotToken)
	if err != nil {
		return err
	}
	log.Printf("Bot authorized with account: %s", client.Username())

	eng := engine.New(engine.Options{
		Catalog:      catalog{c: dvach.New(cfg.DvachURL)},
		Store:        db,
		Sender:       client,
		CheckEvery:   cfg.CheckEvery,
		SendInterval: 3 * time.Second,
		QueueSize:    256,
	})
	if err := eng.Load(ctx); err != nil {
		return err
	}
	log.Printf("Bot started.")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = eng.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		if err := client.Run(ctx, eng); err != nil {
			log.Printf("updates: %v", err)
		}
	}()
	<-ctx.Done()
	wg.Wait()
	log.Printf("Bot stopped.")
	return nil
}
