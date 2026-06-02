package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReloadableHandler(t *testing.T) {
	rh := &reloadableHandler{}
	rh.Store(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		fmt.Fprint(w, "first")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec := httptest.NewRecorder()
	rh.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTeapot, rec.Code)
	require.Equal(t, "first", rec.Body.String())

	rh.Store(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "second")
	}))

	rec = httptest.NewRecorder()
	rh.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "second", rec.Body.String())
}

func TestReloadableHandlerEndToEnd(t *testing.T) {
	rh := &reloadableHandler{}
	rh.Store(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "v1")
	}))
	srv := httptest.NewServer(rh)
	t.Cleanup(srv.Close)

	body := getBody(t, srv.URL)
	require.Equal(t, "v1", body)

	rh.Store(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "v2")
	}))
	body = getBody(t, srv.URL)
	require.Equal(t, "v2", body)
}

func TestWatchFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	require.NoError(t, os.WriteFile(a, []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("b"), 0o644))

	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(a, base, base))
	require.NoError(t, os.Chtimes(b, base, base))

	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		watchFiles(ctx, []string{a, b}, 5*time.Millisecond, func() {
			calls.Add(1)
		})
		close(done)
	}()

	// No changes yet — give the watcher a few ticks to confirm it doesn't fire spuriously.
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int32(0), calls.Load(), "watcher should not fire without changes")

	// Bump mtime of the first file.
	t1 := base.Add(1 * time.Second)
	require.NoError(t, os.Chtimes(a, t1, t1))
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, 5*time.Millisecond)

	// And the second file.
	t2 := base.Add(2 * time.Second)
	require.NoError(t, os.Chtimes(b, t2, t2))
	require.Eventually(t, func() bool { return calls.Load() == 2 }, time.Second, 5*time.Millisecond)

	// Cancelling stops the watcher.
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop after context cancel")
	}

	// Further modifications after cancellation must not fire the callback.
	t3 := base.Add(3 * time.Second)
	require.NoError(t, os.Chtimes(a, t3, t3))
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(2), calls.Load())
}

func TestWatchFilesMissingFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.yaml")

	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go watchFiles(ctx, []string{missing}, 5*time.Millisecond, func() {
		calls.Add(1)
	})

	// A missing file at startup shouldn't crash; creating it later should fire.
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, int32(0), calls.Load())

	require.NoError(t, os.WriteFile(missing, []byte("now exists"), 0o644))
	require.Eventually(t, func() bool { return calls.Load() >= 1 }, time.Second, 5*time.Millisecond)
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}
