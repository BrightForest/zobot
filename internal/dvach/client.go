// Package dvach reads /b/ thread and file lists over HTTPS.
package dvach

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const bodyLimit = 32 << 20

// Thread is one catalog entry. Filtering by subject happens in the engine.
type Thread struct {
	Num     int
	Subject string
}

// Client is an HTTP catalog client. TLS verification stays on.
type Client struct {
	base string
	http *http.Client
}

// New builds a client. baseURL is the site origin, for example https://2ch.su.
func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

// Threads fetches /b/threads.json.
func (c *Client) Threads(ctx context.Context) ([]Thread, error) {
	var payload struct {
		Threads []struct {
			Num     int    `json:"num"`
			Subject string `json:"subject"`
		} `json:"threads"`
	}
	if err := c.getJSON(ctx, c.base+"/b/threads.json", &payload); err != nil {
		return nil, err
	}
	out := make([]Thread, len(payload.Threads))
	for i, th := range payload.Threads {
		out[i] = Thread{Num: th.Num, Subject: th.Subject}
	}
	return out, nil
}

// Pictures fetches file URLs from /b/res/{id}.json.
func (c *Client) Pictures(ctx context.Context, threadID string) ([]string, error) {
	var payload struct {
		Threads []struct {
			Posts []struct {
				Files []struct {
					Path string `json:"path"`
				} `json:"files"`
			} `json:"posts"`
		} `json:"threads"`
	}
	endpoint := c.base + "/b/res/" + url.PathEscape(threadID) + ".json"
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	var links []string
	for _, thread := range payload.Threads {
		for _, post := range thread.Posts {
			for _, file := range post.Files {
				if file.Path == "" {
					continue
				}
				links = append(links, joinURL(c.base, file.Path))
			}
		}
	}
	return links, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %d", endpoint, resp.StatusCode)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, bodyLimit))
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}

func joinURL(base, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	base = strings.TrimRight(base, "/")
	if strings.HasPrefix(path, "/") {
		return base + path
	}
	return base + "/" + path
}
