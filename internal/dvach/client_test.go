package dvach

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestThreadsAndPictures(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/b/threads.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"board":"b","threads":[{"num":7,"subject":"ЗАСМЕЯЛСЯ"},{"num":8,"subject":"other"}]}`))
	})
	mux.HandleFunc("/b/res/7.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"threads":[{"posts":[{"files":[{"path":"/b/src/a.jpg"},{"path":"https://cdn.example/x.png"}]},{"files":[]}]}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL + "/")
	if c.http.Transport != nil {
		t.Fatal("custom transport would risk disabling TLS verify")
	}
	threads, err := c.Threads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 2 || threads[0].Num != 7 || threads[0].Subject != "ЗАСМЕЯЛСЯ" {
		t.Fatalf("threads: %+v", threads)
	}
	links, err := c.Pictures(context.Background(), "7")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0] != srv.URL+"/b/src/a.jpg" || links[1] != "https://cdn.example/x.png" {
		t.Fatalf("links: %#v", links)
	}
}

func TestStatusAndDecodeErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/b/threads.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	})
	mux.HandleFunc("/b/res/1.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	if _, err := c.Threads(context.Background()); err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("status: %v", err)
	}
	if _, err := c.Pictures(context.Background(), "1"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("decode: %v", err)
	}
}
