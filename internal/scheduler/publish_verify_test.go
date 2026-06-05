package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerifyPublishedURLAcceptsHEAD2xx(t *testing.T) {
	var headSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %s, want HEAD", r.Method)
		}
		headSeen = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &Scheduler{httpClient: server.Client()}

	if err := s.verifyPublishedURL(context.Background(), server.URL+"/blog/my-post"); err != nil {
		t.Fatalf("verifyPublishedURL returned error: %v", err)
	}
	if !headSeen {
		t.Fatal("HEAD request was not sent")
	}
}

func TestVerifyPublishedURLFallsBackToGET(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET fallback", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	s := &Scheduler{httpClient: server.Client()}

	if err := s.verifyPublishedURL(context.Background(), server.URL+"/blog/my-post"); err != nil {
		t.Fatalf("verifyPublishedURL returned error: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodHead || methods[1] != http.MethodGet {
		t.Fatalf("methods = %#v", methods)
	}
}

func TestVerifyPublishedURLRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	s := &Scheduler{httpClient: server.Client()}

	if err := s.verifyPublishedURL(context.Background(), server.URL+"/blog/missing"); err == nil {
		t.Fatal("expected non-2xx URL verification to fail")
	}
}

func TestNextPublishRetryAtStopsAfterFifthAttempt(t *testing.T) {
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		attempt int32
		delay   time.Duration
		valid   bool
	}{
		{attempt: 1, delay: 5 * time.Minute, valid: true},
		{attempt: 2, delay: 15 * time.Minute, valid: true},
		{attempt: 3, delay: time.Hour, valid: true},
		{attempt: 4, delay: 6 * time.Hour, valid: true},
		{attempt: 5, valid: false},
	} {
		got := nextPublishRetryAt(now, tc.attempt)
		if got.Valid != tc.valid {
			t.Fatalf("attempt %d valid = %v", tc.attempt, got.Valid)
		}
		if tc.valid && !got.Time.Equal(now.Add(tc.delay)) {
			t.Fatalf("attempt %d retry = %s, want %s", tc.attempt, got.Time, now.Add(tc.delay))
		}
	}
}
