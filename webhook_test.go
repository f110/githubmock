package githubmock

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendWebhook(t *testing.T) {
	var (
		mu     sync.Mutex
		got    []byte
		gotSig string
		gotEv  string
	)
	secret := "s3cret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = body
		gotSig = r.Header.Get("X-Hub-Signature-256")
		gotEv = r.Header.Get("X-GitHub-Event")
		mu.Unlock()
	}))
	defer srv.Close()

	m := NewMock()
	repo := m.Repository("octocat/example")
	repo.Webhook(NewWebhook().URL(srv.URL).Secret(secret).Events("pull_request"))
	repo.PullRequests(NewPullRequest().Number(1).Title("hello").State(PullRequestStateOpen))
	pr := repo.GetPullRequest(1)

	ev := repo.BuildPullRequestEvent("opened", pr, nil)
	deliveries := repo.SendWebhook(t.Context(), "pull_request", ev)
	require.Len(t, deliveries, 1)
	assert.Equal(t, http.StatusOK, deliveries[0].StatusCode)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "pull_request", gotEv)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(got)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	assert.Equal(t, expected, gotSig)
}

func TestWebhookEventFiltering(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()

	m := NewMock()
	repo := m.Repository("octocat/example")
	repo.Webhook(NewWebhook().URL(srv.URL).Events("push"))

	deliveries := repo.SendWebhook(t.Context(), "pull_request", map[string]string{"x": "y"})
	assert.Empty(t, deliveries)
	assert.False(t, hit)
}

func TestWebhookScopedPerRepository(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	m := NewMock()
	repoA := m.Repository("octocat/a")
	repoB := m.Repository("octocat/b")
	repoA.Webhook(NewWebhook().URL(srv.URL).Events("push"))

	repoB.SendWebhook(t.Context(), "push", map[string]string{"x": "y"})
	assert.Equal(t, int32(0), atomic.LoadInt32(&hits), "webhook on repoA must not fire for repoB events")

	repoA.SendWebhook(t.Context(), "push", map[string]string{"x": "y"})
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "webhook on repoA must not fire for repoB events")
}
