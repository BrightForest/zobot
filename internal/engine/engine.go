// Package engine polls the catalog, deduplicates files, and fans them out.
package engine

import (
	"context"
	"errors"
	"log"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/BrightForest/zobot/internal/outbound"
	"github.com/BrightForest/zobot/internal/store"
)

const (
	defaultQueueSize    = 256
	defaultSendInterval = 3 * time.Second
	defaultEvery        = 60 * time.Second
	maxSendAttempts     = 5
)

// Thread is one catalog subject the engine may match.
type Thread struct {
	Num     int
	Subject string
}

// Catalog is the imageboard read API.
type Catalog interface {
	Threads(ctx context.Context) ([]Thread, error)
	Pictures(ctx context.Context, threadID string) ([]string, error)
}

// Store is the subset of persistence the engine needs.
type Store interface {
	ListSubscribers(ctx context.Context) ([]store.Subscriber, error)
	AddSubscriber(ctx context.Context, sub store.Subscriber) error
	SetActive(ctx context.Context, chatID int64, active bool) error
	Activate(ctx context.Context, chatID int64) error
	SetBlocked(ctx context.Context, chatID int64, blocked bool) error
	Regexes(ctx context.Context) ([]string, error)
}

// Options configures an Engine. Zero durations and queue size pick defaults
// (60s poll, 3s between sends, queue of 256).
type Options struct {
	Catalog      Catalog
	Store        Store
	Sender       outbound.Sender
	CheckEvery   time.Duration
	SendInterval time.Duration
	RegexEvery   time.Duration
	QueueSize    int
}

type queueItem struct {
	link   string
	thread string
}

// Engine is the poll/send/command core. Maps are guarded by mu.
type Engine struct {
	catalog      Catalog
	store        Store
	sender       outbound.Sender
	checkEvery   time.Duration
	sendInterval time.Duration
	regexEvery   time.Duration

	queue   chan queueItem
	mu      sync.RWMutex
	threads map[string]struct{}
	images  map[string]string
	sent    map[string]string
	regexes []*regexp.Regexp
	subs    map[int64]*store.Subscriber
	dropped int
}

// New builds an engine. Load must run before Run or PollOnce.
func New(opts Options) *Engine {
	if opts.QueueSize <= 0 {
		opts.QueueSize = defaultQueueSize
	}
	if opts.SendInterval <= 0 {
		opts.SendInterval = defaultSendInterval
	}
	if opts.CheckEvery <= 0 {
		opts.CheckEvery = defaultEvery
	}
	if opts.RegexEvery <= 0 {
		opts.RegexEvery = defaultEvery
	}
	return &Engine{
		catalog:      opts.Catalog,
		store:        opts.Store,
		sender:       opts.Sender,
		checkEvery:   opts.CheckEvery,
		sendInterval: opts.SendInterval,
		regexEvery:   opts.RegexEvery,
		queue:        make(chan queueItem, opts.QueueSize),
		threads:      map[string]struct{}{},
		images:       map[string]string{},
		sent:         map[string]string{},
		subs:         map[int64]*store.Subscriber{},
	}
}

// Load pulls subscribers and regexes. A database error aborts startup.
// A broken regex is skipped. An empty usable set does not crash the process.
func (e *Engine) Load(ctx context.Context) error {
	if err := e.reloadRegex(ctx); err != nil {
		return err
	}
	subs, err := e.store.ListSubscribers(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	for i := range subs {
		s := subs[i]
		e.subs[s.ChatID] = &s
	}
	nReg := len(e.regexes)
	nSub := len(e.subs)
	e.mu.Unlock()
	log.Printf("Subscribers loaded count: %d", nSub)
	log.Printf("regexes ready: %d", nReg)
	return nil
}

// Run blocks until ctx is cancelled and the loops have returned.
func (e *Engine) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		e.pollLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		e.sendLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		e.regexLoop(ctx)
	}()
	wg.Wait()
	return nil
}

func (e *Engine) pollLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		e.PollOnce(ctx)
		if !sleep(ctx, e.checkEvery) {
			return
		}
	}
}

func (e *Engine) regexLoop(ctx context.Context) {
	for {
		if !sleep(ctx, e.regexEvery) {
			return
		}
		if err := e.reloadRegex(ctx); err != nil {
			log.Printf("regex refresh: %v", err)
		}
	}
}

func (e *Engine) sendLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case item := <-e.queue:
			e.fanout(ctx, item.link)
		}
	}
}

// PollOnce refreshes matching threads, enqueues unseen file URLs, and drops
// threads that are no longer on the matched list.
func (e *Engine) PollOnce(ctx context.Context) {
	if e.catalog == nil || ctx.Err() != nil {
		return
	}
	threads, err := e.catalog.Threads(ctx)
	if err != nil {
		log.Printf("threads: %v", err)
		return
	}
	regs := e.regexSnapshot()
	active := make(map[string]struct{})
	for _, th := range threads {
		if !matchAny(regs, th.Subject) {
			continue
		}
		id := strconv.Itoa(th.Num)
		active[id] = struct{}{}
		links, err := e.catalog.Pictures(ctx, id)
		if err != nil {
			log.Printf("thread %s pictures: %v", id, err)
			continue
		}
		e.trackAndEnqueue(id, links)
		log.Printf("Thread %s added to threads list.", id)
	}
	e.dropStale(active)
}

func (e *Engine) trackAndEnqueue(threadID string, links []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.threads[threadID] = struct{}{}
	for _, link := range links {
		if link == "" {
			continue
		}
		e.images[link] = threadID
		if _, ok := e.sent[link]; ok {
			continue
		}
		select {
		case e.queue <- queueItem{link: link, thread: threadID}:
			e.sent[link] = threadID
		default:
			e.dropped++
			log.Printf("queue full, drop %s", link)
		}
	}
}

// dropStale removes threads that were tracked and are absent from active,
// including every URL that belonged to them.
func (e *Engine) dropStale(active map[string]struct{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id := range e.threads {
		if _, ok := active[id]; ok {
			continue
		}
		delete(e.threads, id)
		for link, th := range e.images {
			if th == id {
				delete(e.images, link)
			}
		}
		for link, th := range e.sent {
			if th == id {
				delete(e.sent, link)
			}
		}
		log.Printf("Thread %s was deleted from threads list.", id)
	}
}

func (e *Engine) fanout(ctx context.Context, link string) {
	for _, sub := range e.snapshotActive() {
		if ctx.Err() != nil {
			return
		}
		if !e.deliverable(sub.ChatID) {
			continue
		}
		e.deliver(ctx, sub.ChatID, link)
		if !sleep(ctx, e.sendInterval) {
			return
		}
	}
}

func (e *Engine) deliver(ctx context.Context, chatID int64, text string) {
	if e.sender == nil {
		return
	}
	for attempt := 1; attempt <= maxSendAttempts; attempt++ {
		if ctx.Err() != nil || !e.deliverable(chatID) {
			return
		}
		err := e.sender.Send(ctx, chatID, text)
		if err == nil {
			return
		}
		var limited *outbound.RateLimitError
		if errors.As(err, &limited) {
			wait := limited.RetryAfter
			if wait <= 0 {
				wait = e.sendInterval
			}
			log.Printf("rate limit chat %d, retry after %s (attempt %d)", chatID, wait, attempt)
			if !sleep(ctx, wait) {
				return
			}
			continue
		}
		var forbidden *outbound.ForbiddenError
		if errors.As(err, &forbidden) {
			e.markBlocked(ctx, chatID)
			return
		}
		log.Printf("send to %d: %v", chatID, err)
		return
	}
	log.Printf("send to %d: giving up after %d attempts", chatID, maxSendAttempts)
}

func (e *Engine) markBlocked(ctx context.Context, chatID int64) {
	var username string
	e.mu.Lock()
	if s, ok := e.subs[chatID]; ok {
		s.Blocked = true
		username = s.Username
	}
	e.mu.Unlock()
	log.Printf("%s with chatId %d has disabled bot.", username, chatID)
	if e.store == nil {
		return
	}
	if err := e.store.SetBlocked(ctx, chatID, true); err != nil {
		log.Printf("persist blocked %d: %v", chatID, err)
	}
}

// HandleCommand implements the Telegram command replies.
func (e *Engine) HandleCommand(ctx context.Context, chatID int64, username, command string) string {
	e.ensureSubscriber(ctx, chatID, username)
	switch command {
	case "start":
		return "Жмём /disco чтобы начать залив мемасов."
	case "help":
		return "Можно прислать команду /pause, чтобы остановить поток шитшторма.\nКоманда /disco возобновит залив мемасов."
	case "pause":
		e.mu.Lock()
		if s, ok := e.subs[chatID]; ok {
			s.IsActive = false
		}
		e.mu.Unlock()
		if err := e.store.SetActive(ctx, chatID, false); err != nil {
			log.Printf("pause %d: %v", chatID, err)
		}
		log.Printf("%s paused bot.", username)
		return "Мемсный шторм остановлен."
	case "disco":
		e.mu.Lock()
		if s, ok := e.subs[chatID]; ok {
			s.IsActive = true
			s.Blocked = false
		}
		e.mu.Unlock()
		if err := e.store.Activate(ctx, chatID); err != nil {
			log.Printf("disco %d: %v", chatID, err)
		}
		log.Printf("%s activated bot.", username)
		return "Мемсный шторм включён."
	default:
		return "Неизвестная команда."
	}
}

func (e *Engine) ensureSubscriber(ctx context.Context, chatID int64, username string) {
	e.mu.Lock()
	if s, ok := e.subs[chatID]; ok {
		if username != "" {
			s.Username = username
		}
		e.mu.Unlock()
		return
	}
	s := &store.Subscriber{ChatID: chatID, Username: username, IsActive: false}
	e.subs[chatID] = s
	e.mu.Unlock()
	log.Printf("New subscriber added with username: %s", username)
	if err := e.store.AddSubscriber(ctx, *s); err != nil {
		log.Printf("add subscriber %d: %v", chatID, err)
	}
}

func (e *Engine) reloadRegex(ctx context.Context) error {
	raws, err := e.store.Regexes(ctx)
	if err != nil {
		return err
	}
	compiled := make([]*regexp.Regexp, 0, len(raws))
	for _, raw := range raws {
		re, err := regexp.Compile(raw)
		if err != nil {
			log.Printf("skip regex %q: %v", raw, err)
			continue
		}
		compiled = append(compiled, re)
	}
	if len(compiled) == 0 {
		e.mu.RLock()
		keep := len(e.regexes) > 0
		e.mu.RUnlock()
		if keep {
			log.Printf("regex refresh empty, keeping previous")
			return nil
		}
		log.Printf("no usable regexes")
		return nil
	}
	e.mu.Lock()
	e.regexes = compiled
	e.mu.Unlock()
	return nil
}

func (e *Engine) regexSnapshot() []*regexp.Regexp {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]*regexp.Regexp, len(e.regexes))
	copy(out, e.regexes)
	return out
}

func (e *Engine) snapshotActive() []store.Subscriber {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]store.Subscriber, 0, len(e.subs))
	for _, s := range e.subs {
		if s.IsActive && !s.Blocked {
			out = append(out, *s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChatID < out[j].ChatID })
	return out
}

func (e *Engine) deliverable(chatID int64) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.subs[chatID]
	return ok && s.IsActive && !s.Blocked
}

func matchAny(regs []*regexp.Regexp, subject string) bool {
	for _, re := range regs {
		if re.MatchString(subject) {
			return true
		}
	}
	return false
}

func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
