package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/BrightForest/zobot/internal/outbound"
	"github.com/BrightForest/zobot/internal/store"
)

type memStore struct {
	mu       sync.Mutex
	subs     map[int64]store.Subscriber
	regex    []string
	regexErr error
}

func newMem(regex ...string) *memStore {
	return &memStore{subs: map[int64]store.Subscriber{}, regex: regex}
}

func (m *memStore) ListSubscribers(context.Context) ([]store.Subscriber, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.Subscriber, 0, len(m.subs))
	for _, s := range m.subs {
		out = append(out, s)
	}
	return out, nil
}

func (m *memStore) AddSubscriber(_ context.Context, s store.Subscriber) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.subs[s.ChatID]; ok {
		return nil
	}
	m.subs[s.ChatID] = s
	return nil
}

func (m *memStore) SetActive(_ context.Context, chatID int64, active bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.subs[chatID]
	s.ChatID = chatID
	s.IsActive = active
	m.subs[chatID] = s
	return nil
}

func (m *memStore) Activate(_ context.Context, chatID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.subs[chatID]
	s.ChatID = chatID
	s.IsActive = true
	s.Blocked = false
	m.subs[chatID] = s
	return nil
}

func (m *memStore) SetBlocked(_ context.Context, chatID int64, blocked bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.subs[chatID]
	s.ChatID = chatID
	s.Blocked = blocked
	m.subs[chatID] = s
	return nil
}

func (m *memStore) Regexes(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.regexErr != nil {
		return nil, m.regexErr
	}
	out := append([]string(nil), m.regex...)
	return out, nil
}

type fakeCat struct {
	threads []Thread
	pics    map[string][]string
	picErr  map[string]error
}

func (f fakeCat) Threads(context.Context) ([]Thread, error) { return f.threads, nil }

func (f fakeCat) Pictures(_ context.Context, id string) ([]string, error) {
	if err := f.picErr[id]; err != nil {
		return nil, err
	}
	return f.pics[id], nil
}

type scriptedSender struct {
	mu    sync.Mutex
	calls []int64
	texts []string
	fn    func(chatID int64, n int) error
}

func (s *scriptedSender) Send(_ context.Context, chatID int64, text string) error {
	s.mu.Lock()
	s.calls = append(s.calls, chatID)
	s.texts = append(s.texts, text)
	n := 0
	for _, id := range s.calls {
		if id == chatID {
			n++
		}
	}
	fn := s.fn
	s.mu.Unlock()
	if fn != nil {
		return fn(chatID, n)
	}
	return nil
}

func testEngine(t *testing.T, cat Catalog, st Store, sender outbound.Sender, queue int) *Engine {
	t.Helper()
	e := New(Options{
		Catalog:      cat,
		Store:        st,
		Sender:       sender,
		QueueSize:    queue,
		CheckEvery:   time.Hour,
		SendInterval: time.Hour,
		RegexEvery:   time.Hour,
	})
	e.sendInterval = 0
	return e
}

func TestPollDedupAndCleanup(t *testing.T) {
	st := newMem(`(?i).*засме.*`)
	cat := &fakeCat{
		threads: []Thread{{Num: 1, Subject: "ЗАСМЕЯЛСЯ"}, {Num: 2, Subject: "other"}},
		pics:    map[string][]string{"1": {"https://img/a", "https://img/a", "https://img/b"}},
	}
	e := testEngine(t, cat, st, &scriptedSender{}, 8)
	if err := e.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.PollOnce(context.Background())
	if len(e.sent) != 2 || len(e.threads) != 1 {
		t.Fatalf("sent=%d threads=%d", len(e.sent), len(e.threads))
	}
	if got := len(e.queue); got != 2 {
		t.Fatalf("queue len %d", got)
	}

	e.PollOnce(context.Background())
	if len(e.queue) != 2 {
		t.Fatal("dedup should not enqueue again")
	}

	cat.threads = []Thread{{Num: 3, Subject: "засме"}}
	cat.pics["3"] = []string{"https://img/c"}
	e.PollOnce(context.Background())
	if _, ok := e.threads["1"]; ok {
		t.Fatal("stale thread kept")
	}
	if _, ok := e.images["https://img/a"]; ok || e.sent["https://img/a"] != "" {
		t.Fatal("stale urls kept")
	}
	if e.sent["https://img/c"] != "3" || e.images["https://img/c"] != "3" {
		t.Fatalf("new thread missing: images=%v sent=%v", e.images, e.sent)
	}
}

func TestQueueDropIsRetried(t *testing.T) {
	st := newMem(".*")
	cat := &fakeCat{
		threads: []Thread{{Num: 9, Subject: "x"}},
		pics:    map[string][]string{"9": {"https://img/1", "https://img/2"}},
	}
	e := testEngine(t, cat, st, &scriptedSender{}, 1)
	if err := e.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.PollOnce(context.Background())
	if e.dropped != 1 || len(e.sent) != 1 {
		t.Fatalf("dropped=%d sent=%d", e.dropped, len(e.sent))
	}
	<-e.queue
	e.PollOnce(context.Background())
	if len(e.sent) != 2 || len(e.queue) != 1 {
		t.Fatalf("retry failed sent=%v queue=%d", e.sent, len(e.queue))
	}
}

func TestBadRegexDoesNotPanic(t *testing.T) {
	st := newMem(`(?i)ok`, `[unterminated`)
	e := testEngine(t, fakeCat{}, st, &scriptedSender{}, 1)
	if err := e.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.regexes) != 1 {
		t.Fatalf("regexes: %d", len(e.regexes))
	}
	st.mu.Lock()
	st.regex = []string{`(bad`}
	st.mu.Unlock()
	if err := e.reloadRegex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.regexes) != 1 || e.regexes[0].String() != `(?i)ok` {
		t.Fatalf("previous regex dropped: %v", e.regexes)
	}
}

func TestPictureErrorKeepsThread(t *testing.T) {
	st := newMem(".*")
	cat := &fakeCat{
		threads: []Thread{{Num: 4, Subject: "x"}},
		pics:    map[string][]string{"4": {"https://img/keep"}},
	}
	e := testEngine(t, cat, st, &scriptedSender{}, 4)
	if err := e.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.PollOnce(context.Background())
	cat.picErr = map[string]error{"4": errors.New("boom")}
	e.PollOnce(context.Background())
	if _, ok := e.threads["4"]; !ok || e.sent["https://img/keep"] != "4" {
		t.Fatal("transient picture error wiped the thread")
	}
}

func TestFanoutOrderBlockAndRateLimit(t *testing.T) {
	st := newMem(".*")
	st.subs[30] = store.Subscriber{ChatID: 30, Username: "late", IsActive: true}
	st.subs[10] = store.Subscriber{ChatID: 10, Username: "early", IsActive: true}
	st.subs[20] = store.Subscriber{ChatID: 20, Username: "off", IsActive: false}
	st.subs[40] = store.Subscriber{ChatID: 40, Username: "ban", IsActive: true, Blocked: true}

	sender := &scriptedSender{fn: func(chatID int64, n int) error {
		if chatID == 10 && n == 1 {
			return &outbound.RateLimitError{RetryAfter: time.Millisecond}
		}
		if chatID == 30 {
			return &outbound.ForbiddenError{Err: errors.New("Forbidden")}
		}
		return nil
	}}
	e := testEngine(t, fakeCat{}, st, sender, 2)
	if err := e.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.fanout(context.Background(), "https://img/z")

	if len(sender.calls) != 3 || sender.calls[0] != 10 || sender.calls[1] != 10 || sender.calls[2] != 30 {
		t.Fatalf("calls: %v", sender.calls)
	}
	if !e.subs[30].Blocked || e.subs[10].Blocked {
		t.Fatalf("blocked flags: 30=%v 10=%v", e.subs[30].Blocked, e.subs[10].Blocked)
	}
	st.mu.Lock()
	if !st.subs[30].Blocked {
		t.Fatal("blocked not persisted")
	}
	st.mu.Unlock()

	e.fanout(context.Background(), "https://img/next")
	if len(sender.calls) != 4 || sender.calls[3] != 10 {
		t.Fatalf("second fanout should skip blocked user, calls=%v", sender.calls)
	}
}

func TestCommands(t *testing.T) {
	st := newMem(".*")
	e := testEngine(t, fakeCat{}, st, &scriptedSender{}, 1)
	if err := e.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if got := e.HandleCommand(ctx, 7, "neo", "start"); got != "Жмём /disco чтобы начать залив мемасов." {
		t.Fatal(got)
	}
	if e.subs[7].IsActive {
		t.Fatal("start must not enable delivery")
	}
	if got := e.HandleCommand(ctx, 7, "neo", "disco"); got != "Мемсный шторм включён." || !e.subs[7].IsActive {
		t.Fatal("disco")
	}
	e.subs[7].Blocked = true
	if got := e.HandleCommand(ctx, 7, "neo", "disco"); got != "Мемсный шторм включён." || e.subs[7].Blocked || !e.subs[7].IsActive {
		t.Fatal("disco should clear blocked")
	}
	if got := e.HandleCommand(ctx, 7, "neo", "pause"); got != "Мемсный шторм остановлен." || e.subs[7].IsActive {
		t.Fatal("pause")
	}
	if got := e.HandleCommand(ctx, 7, "neo", "nope"); got != "Неизвестная команда." {
		t.Fatal(got)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.subs[7].IsActive || st.subs[7].Blocked {
		t.Fatalf("store: %+v", st.subs[7])
	}
}
