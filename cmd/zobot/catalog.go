package main

import (
	"context"

	"github.com/BrightForest/zobot/internal/dvach"
	"github.com/BrightForest/zobot/internal/engine"
)

// catalog adapts the imageboard client to the engine port.
type catalog struct {
	c *dvach.Client
}

func (c catalog) Threads(ctx context.Context) ([]engine.Thread, error) {
	list, err := c.c.Threads(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]engine.Thread, len(list))
	for i, th := range list {
		out[i] = engine.Thread{Num: th.Num, Subject: th.Subject}
	}
	return out, nil
}

func (c catalog) Pictures(ctx context.Context, threadID string) ([]string, error) {
	return c.c.Pictures(ctx, threadID)
}
