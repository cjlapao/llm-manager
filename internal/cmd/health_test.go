package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitForHealthy_Success(t *testing.T) {
	// Mock server that returns 200 immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := waitForHealthy(ctx, server.URL)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestWaitForHealthy_SuccessAfterRetries(t *testing.T) {
	// Mock server that fails a few times then succeeds
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.URL.Path == "/health" {
			if attempts < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Need 3 attempts * 3s interval = 9s minimum, use 12s to be safe
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	err := waitForHealthy(ctx, server.URL)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if attempts < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", attempts)
	}
}

func TestWaitForHealthy_Timeout(t *testing.T) {
	// Mock server that never returns 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := waitForHealthy(ctx, server.URL)
	if err == nil {
		t.Fatal("expected error on timeout, got nil")
	}
}

func TestWaitForHealthy_TransientFailure(t *testing.T) {
	// Mock server that fails once, then succeeds
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.URL.Path == "/health" {
			if attempts == 1 {
				// Simulate a transient connection error by closing the connection
				// We'll just return an error status to simulate transient failure
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Need 2 attempts * 3s interval = 6s minimum, use 10s to be safe
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := waitForHealthy(ctx, server.URL)
	if err != nil {
		t.Fatalf("expected nil error after transient failure, got: %v", err)
	}
	if attempts < 2 {
		t.Fatalf("expected at least 2 attempts (1 failure + 1 success), got %d", attempts)
	}
}

func TestWaitForHealthy_NoDeadline(t *testing.T) {
	// Context without deadline should return an error immediately
	ctx := context.Background()
	err := waitForHealthy(ctx, "http://localhost:8080")
	if err == nil {
		t.Fatal("expected error for context without deadline, got nil")
	}
}
