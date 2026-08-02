package githubmock

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/go-github/v83/github"
	"github.com/stretchr/testify/assert"
)

type WebhookDelivery struct {
	URL        string
	StatusCode int
	Err        error
}

type Repository struct {
	mu           sync.Mutex
	pullRequests []*PullRequest
	issues       []*Issue
	tags         []*Tag
	commits      []*Commit
	webhooks     []*Webhook
	stacks       []*Stack

	headCommit *Commit
	rootCommit *Commit

	ghRepository *github.Repository

	logger *slog.Logger
}

func newRepository() *Repository {
	return &Repository{ghRepository: &github.Repository{}}
}

func (r *Repository) FullName() string {
	return r.ghRepository.GetFullName()
}

func (r *Repository) GetCommits() []*Commit {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Commit, len(r.commits))
	copy(out, r.commits)
	return out
}

func (r *Repository) GetCommit(sha string) *Commit {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.commits {
		if c.ghCommit.GetSHA() == sha {
			return c
		}
	}
	return nil
}

func (r *Repository) Webhook(w *Webhook) *Webhook {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.webhooks = append(r.webhooks, w)
	return w
}

func (r *Repository) Webhooks() []*Webhook {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Webhook, len(r.webhooks))
	copy(out, r.webhooks)
	return out
}

func (r *Repository) AssertPullRequest(t *testing.T, number int) *PullRequest {
	t.Helper()
	for _, v := range r.pullRequests {
		if v.ghPullRequest.GetNumber() == number {
			return v
		}
	}

	assert.Failf(t, "Pull request is not found", "pull request %d is not found", number)
	return nil
}

func (r *Repository) PullRequests(pullRequests ...*PullRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range pullRequests {
		r.pullRequests = append(r.pullRequests, v)
		if v.ghPullRequest.GetNumber() == 0 {
			v.ghPullRequest.Number = new(r.nextIndex())
		}
		if v.ghPullRequest.Base == nil {
			v.ghPullRequest.Base = &github.PullRequestBranch{}
		}
		v.ghPullRequest.Base.Repo = r.ghRepository
		if v.headRepo == nil {
			v.headRepo = r
		}
	}
}

func (r *Repository) GetPullRequest(num int) *PullRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.pullRequests {
		if v.ghPullRequest.GetNumber() == num {
			return v
		}
	}
	return nil
}

func (r *Repository) GetPullRequests() []*PullRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.pullRequests
}

func (r *Repository) Stacks(stacks ...*Stack) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range stacks {
		if v.number == 0 {
			v.number = r.nextStackIndex()
		}
		if v.id == 0 {
			v.id = v.number
		}
		if v.url == "" {
			v.url = fmt.Sprintf("%s/stacks/%d", r.ghRepository.GetHTMLURL(), v.number)
		}
		r.stacks = append(r.stacks, v)
	}
}

func (r *Repository) GetStacks() []*Stack {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Stack, len(r.stacks))
	copy(out, r.stacks)
	return out
}

func (r *Repository) GetStack(number int) *Stack {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.stacks {
		if v.number == number {
			return v
		}
	}
	return nil
}

// stackContaining returns the stack that holds the given pull request number,
// or nil when the pull request is not part of any stack.
func (r *Repository) stackContaining(prNumber int) *Stack {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.stacks {
		for _, pr := range s.pullRequests {
			if pr.GetNumber() == prNumber {
				return s
			}
		}
	}
	return nil
}

func (r *Repository) appendToStack(s *Stack, prs []*PullRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s.pullRequests = append(s.pullRequests, prs...)
}

func (r *Repository) removeStack(number int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stacks := make([]*Stack, 0, len(r.stacks))
	for _, s := range r.stacks {
		if s.number != number {
			stacks = append(stacks, s)
		}
	}
	r.stacks = stacks
}

func (r *Repository) nextStackIndex() int {
	var last int
	for _, v := range r.stacks {
		if v.number > last {
			last = v.number
		}
	}
	return last + 1
}

func (r *Repository) Issues(issues ...*Issue) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range issues {
		r.issues = append(r.issues, v)
		if v.ghIssue.GetNumber() == 0 {
			v.ghIssue.Number = new(r.nextIndex())
		}
		v.ghIssue.Repository = r.ghRepository
	}
}

func (r *Repository) GetIssue(num int) *Issue {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.issues {
		if v.ghIssue.GetNumber() == num {
			return v
		}
	}
	return nil
}

func (r *Repository) GetIssues() []*Issue {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.issues
}

func (r *Repository) Commits(commits ...*Commit) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	newCommits := append(r.commits, commits...)
	var rootCommit, headCommit *Commit
	for _, v := range newCommits {
		if len(v.parents) == 0 {
			if rootCommit != nil {
				return errors.New("multiple root commits are found")
			}
			rootCommit = v
		}
		if v.isHead {
			if headCommit != nil {
				return errors.New("multiple head commits are found")
			}
			headCommit = v
		}

		if v.ghCommit.GetSHA() == "" {
			v.ghCommit.SHA = new(newHash())
		}
		v.ghCommit.Tree = &github.Tree{SHA: new(v.files[0].sha)}
		for _, f := range v.files {
			if f.Name == "" {
				continue
			}

			name := f.Name
			if name[0] == '/' {
				name = name[1:]
			}
			s := strings.Split(name, "/")
			f.Name = name
			var dirs []string
			if len(s) > 1 {
				dirs = s[:len(s)-1]
			}
			for i := 1; i <= len(dirs); i++ {
				dir := strings.Join(dirs[:i], "/")
				v.addDir(dir)
			}
			v.addFile(f)
		}
	}

	r.commits = newCommits
	for _, v := range r.commits {
		if len(v.parents) == 0 {
			r.rootCommit = v
		}
		if v.isHead {
			r.headCommit = v
		}
	}
	return nil
}

func (r *Repository) Tags(tags ...*Tag) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tags = append(r.tags, tags...)
}

func (r *Repository) DefaultBranch(v string) {
	r.ghRepository.DefaultBranch = &v
}

func (r *Repository) NextIndex() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.nextIndex()
}

func (r *Repository) nextIndex() int {
	var lastIndex int
	for _, v := range r.pullRequests {
		if v.ghPullRequest.GetNumber() > lastIndex {
			lastIndex = v.ghPullRequest.GetNumber()
		}
	}
	for _, v := range r.issues {
		if v.ghIssue.GetNumber() > lastIndex {
			lastIndex = v.ghIssue.GetNumber()
		}
	}

	return lastIndex + 1
}

func (r *Repository) SendWebhook(ctx context.Context, event string, payload any) []*WebhookDelivery {
	body, err := json.Marshal(payload)
	if err != nil {
		return []*WebhookDelivery{{Err: err}}
	}

	var deliveries []*WebhookDelivery
	for _, w := range r.Webhooks() {
		if !w.accepts(event) {
			continue
		}
		deliveries = append(deliveries, r.deliverWebhook(ctx, w, event, body))
	}
	return deliveries
}

func (r *Repository) deliverWebhook(ctx context.Context, w *Webhook, event string, body []byte) *WebhookDelivery {
	d := &WebhookDelivery{URL: w.url}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		d.Err = err
		return d
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "githubmock-webhook")
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-GitHub-Delivery", newHash()[:36])
	if w.secret != "" {
		mac := hmac.New(sha256.New, []byte(w.secret))
		mac.Write(body)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("webhook delivery failed", slog.String("url", w.url), slog.Any("err", err))
		}
		d.Err = err
		return d
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	d.StatusCode = resp.StatusCode
	return d
}

func (r *Repository) BuildPullRequestEvent(action string, pr *PullRequest, sender *User) *github.PullRequestEvent {
	if pr == nil {
		return nil
	}
	ev := &github.PullRequestEvent{
		Action:      &action,
		Number:      pr.ghPullRequest.Number,
		PullRequest: pr.ghPullRequest,
		Repo:        r.ghRepository,
	}
	if sender != nil {
		ev.Sender = sender.ghUser
	} else if pr.ghPullRequest.User != nil {
		ev.Sender = pr.ghPullRequest.User
	}
	return ev
}

func (r *Repository) BuildPushEvent(ref string, commit *Commit, sender *User) *github.PushEvent {
	if commit == nil {
		return nil
	}
	ev := &github.PushEvent{
		Ref:   &ref,
		Head:  commit.ghCommit.SHA,
		After: commit.ghCommit.SHA,
		Repo: &github.PushEventRepository{
			Name:          r.ghRepository.Name,
			FullName:      r.ghRepository.FullName,
			Owner:         r.ghRepository.Owner,
			HTMLURL:       r.ghRepository.HTMLURL,
			CloneURL:      r.ghRepository.CloneURL,
			MasterBranch:  r.ghRepository.DefaultBranch,
			DefaultBranch: r.ghRepository.DefaultBranch,
		},
		HeadCommit: &github.HeadCommit{
			ID:        commit.ghCommit.SHA,
			SHA:       commit.ghCommit.SHA,
			Timestamp: &github.Timestamp{Time: time.Now()},
		},
	}
	if commit.ghCommit.Tree != nil {
		ev.HeadCommit.TreeID = commit.ghCommit.Tree.SHA
	}
	if sender != nil {
		ev.Sender = sender.ghUser
	}
	if msg := commit.ghCommit.Message; msg != nil {
		ev.HeadCommit.Message = msg
	}
	if before := firstParentSHA(commit); before != "" {
		ev.Before = &before
	}
	return ev
}

type Mock struct {
	Logger    *slog.Logger
	Scheme    string
	Host      string
	Port      int
	GitHubURL string
	// GitURL is the base URL for repository.clone_url. When empty, GitHubURL is
	// used (matching GitHub, where clone_url shares the host with html_url). Set
	// it when the git server is reachable at a different address than the API.
	GitURL string

	mu           sync.Mutex
	repositories map[string]*Repository
	users        map[string]*User
	teams        map[string]*Team
}

func NewMock() *Mock {
	return &Mock{
		Logger:       slog.New(slog.DiscardHandler),
		GitHubURL:    "https://github.com",
		repositories: make(map[string]*Repository),
		users:        make(map[string]*User),
		teams:        make(map[string]*Team),
	}
}

func (m *Mock) Repository(name string) *Repository {
	if strings.Count(name, "/") != 1 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if r, ok := m.repositories[name]; ok {
		return r
	}

	username := name[:strings.Index(name, "/")]
	u, ok := m.users[username]
	if !ok || u == nil {
		u = m.newUser(username)
	}

	r := newRepository()
	r.ghRepository.Name = new(name[strings.Index(name, "/")+1:])
	r.ghRepository.FullName = new(name)
	r.ghRepository.Owner = u.ghUser
	r.ghRepository.HTMLURL = new(strings.TrimRight(m.GitHubURL, "/") + "/" + name)
	cloneBase := m.GitHubURL
	if m.GitURL != "" {
		cloneBase = m.GitURL
	}
	r.ghRepository.CloneURL = new(strings.TrimRight(cloneBase, "/") + "/" + name + ".git")
	r.logger = m.Logger
	m.repositories[name] = r
	return r
}

func (m *Mock) User(login string) *User {
	if strings.Index(login, "/") != -1 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.newUser(login)
}

func (m *Mock) newUser(name string) *User {
	if u, ok := m.users[name]; ok {
		return u
	}
	u := NewUser().Login(name)
	m.users[name] = u
	return u
}

func (m *Mock) Team(slug string) *Team {
	if strings.Index(slug, "/") == -1 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.teams[slug]; ok {
		return t
	}
	t := NewTeam().Slug(slug)
	m.teams[slug] = t
	return t
}

func (m *Mock) Transport() http.RoundTripper {
	mux := http.NewServeMux()
	m.RegisterHandler(mux)

	return &transport{handler: mux}
}

func (m *Mock) Repositories() []*Repository {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Repository, 0, len(m.repositories))
	for _, r := range m.repositories {
		out = append(out, r)
	}
	return out
}

func (m *Mock) LookupRepository(name string) *Repository {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.repositories[name]
}

func (m *Mock) RegisterHandler(mux *http.ServeMux) {
	m.registerMultiplexer(mux)
}

func (m *Mock) registerMultiplexer(mux *http.ServeMux) {
	m.registerPullRequestService(mux)
	m.registerStacksService(mux)
	m.registerIssuesService(mux)
	m.registerRepositoriesService(mux)
	m.registerGitService(mux)
	m.registerUsersService(mux)
	m.registerTeamsService(mux)
	m.registerGraphQLService(mux)
	m.registerHandleFunc(mux, "/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	})
}

func (m *Mock) registerHandleFunc(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			err := recover()
			if err != nil && err == escape {
				return
			}
			if err != nil {
				panic(err)
			}
		}()

		handler(w, req)
	})
}

func (m *Mock) registerPullRequestService(mux *http.ServeMux) {
	// Get a pull request
	// GET /repos/octocat/example/pulls/1
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/pulls/{number}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		pr := r.GetPullRequest(num)
		m.jsonResponse(req.Context(), w, http.StatusOK, pr.ghPullRequest)
	})
	// List pull requests
	// GET /repos/octocat/example/pulls
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/pulls", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}

		// Filtering
		state := "open"
		if req.URL.Query().Get("state") != "" {
			state = req.URL.Query().Get("state")
		}
		var prs []*github.PullRequest
		for _, v := range r.GetPullRequests() {
			if v.ghPullRequest.GetState() == state || state == "all" {
				prs = append(prs, v.ghPullRequest)
			}
		}

		// Sorting
		sort := "created"
		if req.URL.Query().Get("sort") != "" {
			sort = req.URL.Query().Get("sort")
		}
		var direction string
		switch sort {
		case "created":
			direction = "desc"
		default:
			direction = "asc"
		}
		if req.URL.Query().Get("direction") != "" {
			direction = req.URL.Query().Get("direction")
		}
		slices.SortFunc(prs, func(a, b *github.PullRequest) int {
			switch sort {
			case "created":
				return a.CreatedAt.Time.Compare(b.CreatedAt.Time)
			case "updated":
				return a.UpdatedAt.Time.Compare(b.UpdatedAt.Time)
			default:
				return a.CreatedAt.Time.Compare(b.CreatedAt.Time)
			}
		})
		if direction == "desc" {
			slices.Reverse(prs)
		}

		m.jsonWithPaginationResponse(req.Context(), w, req, http.StatusOK, prs)
	})
	// Create a pull request
	// POST /repos/octocat/example/pulls
	m.registerHandleFunc(mux, "POST /repos/{owner}/{repo}/pulls", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		var reqPR github.NewPullRequest
		if err := json.NewDecoder(req.Body).Decode(&reqPR); err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}

		pr := &github.PullRequest{
			Number: new(r.NextIndex()),
			Title:  reqPR.Title,
			Body:   reqPR.Body,
			Head: &github.PullRequestBranch{
				Ref: reqPR.Head,
			},
			Base: &github.PullRequestBranch{
				Ref: reqPR.Base,
			},
		}
		r.PullRequests(&PullRequest{ghPullRequest: pr})
		m.jsonResponse(req.Context(), w, http.StatusOK, pr)
	})
	// Update a pull request
	// PATCH /repos/octocat/example/pulls/1
	m.registerHandleFunc(mux, "PATCH /repos/{owner}/{repo}/pulls/{number}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		pr := r.GetPullRequest(num)
		if pr == nil {
			m.notFoundResponse(req.Context(), w)
		}
		var reqPR struct {
			Title               *string `json:"title,omitempty"`
			Body                *string `json:"body,omitempty"`
			State               *string `json:"state,omitempty"`
			Base                *string `json:"base,omitempty"`
			MaintainerCanModify *bool   `json:"maintainer_can_modify,omitempty"`
		}
		if err := json.NewDecoder(req.Body).Decode(&reqPR); err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
			return
		}

		if reqPR.Title != nil {
			pr.ghPullRequest.Title = reqPR.Title
		}
		if reqPR.Body != nil {
			pr.ghPullRequest.Body = reqPR.Body
		}
		if reqPR.State != nil {
			pr.ghPullRequest.State = reqPR.State
		}
		if reqPR.Base != nil {
			if pr.ghPullRequest.Base == nil {
				pr.ghPullRequest.Base = &github.PullRequestBranch{}
			}
			pr.ghPullRequest.Base.Ref = reqPR.Base
		}
		if reqPR.MaintainerCanModify != nil {
			pr.ghPullRequest.MaintainerCanModify = reqPR.MaintainerCanModify
		}

		m.jsonResponse(req.Context(), w, http.StatusOK, pr.ghPullRequest)
	})
	// Create a new comment
	// POST /repos/octocat/example/pulls/1/comments
	m.registerHandleFunc(mux, "POST /repos/{owner}/{repo}/pulls/{number}/comments", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		pr := r.GetPullRequest(num)
		if pr == nil {
			m.notFoundResponse(req.Context(), w)
		}
		var comment github.PullRequestComment
		if err := json.NewDecoder(req.Body).Decode(&comment); err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}

		pr.comments = append(pr.comments, &comment)
		m.jsonResponse(req.Context(), w, http.StatusOK, comment)
	})
	// List reviews
	// GET /repos/octocat/example/pulls/1/reviews
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/pulls/{number}/reviews", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		pr := r.GetPullRequest(num)
		m.jsonResponse(req.Context(), w, http.StatusOK, pr.reviews)
	})
}

type stackResponse struct {
	ID           int               `json:"id"`
	Number       int               `json:"number"`
	NodeID       string            `json:"node_id"`
	URL          string            `json:"url"`
	Base         stackBaseResponse `json:"base"`
	Open         bool              `json:"open"`
	CreatedAt    string            `json:"created_at"`
	PullRequests []stackPRResponse `json:"pull_requests"`
}

type stackBaseResponse struct {
	Ref string `json:"ref"`
}

type stackPRResponse struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
}

func (s *Stack) toResponse() *stackResponse {
	prs := make([]stackPRResponse, len(s.pullRequests))
	for i, pr := range s.pullRequests {
		state := pr.GetState()
		if state == "" {
			state = "open"
		}
		prs[i] = stackPRResponse{Number: pr.GetNumber(), State: state, Draft: pr.ghPullRequest.GetDraft()}
	}
	return &stackResponse{
		ID:           s.id,
		Number:       s.number,
		NodeID:       s.nodeID,
		URL:          s.url,
		Base:         stackBaseResponse{Ref: s.baseRef},
		Open:         s.open,
		CreatedAt:    s.createdAt.Format(time.RFC3339),
		PullRequests: prs,
	}
}

type stackRequest struct {
	PullRequests []int `json:"pull_requests"`
}

func (m *Mock) registerStacksService(mux *http.ServeMux) {
	// List stacks
	// GET /repos/octocat/example/stacks
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/stacks", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		res := make([]*stackResponse, 0)
		for _, s := range r.GetStacks() {
			res = append(res, s.toResponse())
		}
		m.jsonResponse(req.Context(), w, http.StatusOK, res)
	})
	// Create a stack
	// POST /repos/octocat/example/stacks
	m.registerHandleFunc(mux, "POST /repos/{owner}/{repo}/stacks", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		var reqBody stackRequest
		if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		if len(reqBody.PullRequests) < 2 {
			m.errResponse(req.Context(), w, http.StatusUnprocessableEntity, "Stack must contain at least two pull requests")
		}
		prs := make([]*PullRequest, 0, len(reqBody.PullRequests))
		for _, n := range reqBody.PullRequests {
			if r.stackContaining(n) != nil {
				m.errResponse(req.Context(), w, http.StatusUnprocessableEntity, fmt.Sprintf("Pull request #%d is already stacked", n))
			}
			pr := r.GetPullRequest(n)
			if pr == nil {
				m.errResponse(req.Context(), w, http.StatusUnprocessableEntity, fmt.Sprintf("Pull request #%d does not exist", n))
				return
			}
			prs = append(prs, pr)
		}
		s := &Stack{pullRequests: prs, open: true, createdAt: time.Now()}
		r.Stacks(s)
		m.jsonResponse(req.Context(), w, http.StatusCreated, s.toResponse())
	})
	// Get a stack
	// GET /repos/octocat/example/stacks/1
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/stacks/{number}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		s := r.GetStack(num)
		if s == nil {
			m.notFoundResponse(req.Context(), w)
		}
		m.jsonResponse(req.Context(), w, http.StatusOK, s.toResponse())
	})
	// Add pull requests to a stack
	// POST /repos/octocat/example/stacks/1/add
	m.registerHandleFunc(mux, "POST /repos/{owner}/{repo}/stacks/{number}/add", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		s := r.GetStack(num)
		if s == nil {
			m.notFoundResponse(req.Context(), w)
		}
		var reqBody stackRequest
		if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		prs := make([]*PullRequest, 0, len(reqBody.PullRequests))
		for _, n := range reqBody.PullRequests {
			if containing := r.stackContaining(n); containing != nil && containing != s {
				m.errResponse(req.Context(), w, http.StatusUnprocessableEntity, fmt.Sprintf("Pull request #%d is already stacked", n))
			}
			pr := r.GetPullRequest(n)
			if pr == nil {
				m.errResponse(req.Context(), w, http.StatusUnprocessableEntity, fmt.Sprintf("Pull request #%d does not exist", n))
				return
			}
			prs = append(prs, pr)
		}
		r.appendToStack(s, prs)
		m.jsonResponse(req.Context(), w, http.StatusOK, s.toResponse())
	})
	// Remove unmerged pull requests from a stack
	// POST /repos/octocat/example/stacks/1/unstack
	m.registerHandleFunc(mux, "POST /repos/{owner}/{repo}/stacks/{number}/unstack", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		s := r.GetStack(num)
		if s == nil {
			m.notFoundResponse(req.Context(), w)
		}
		// All pull requests are treated as unmerged, so the stack is dissolved.
		r.removeStack(num)
		m.noContentResponse(w)
	})
}

func (m *Mock) registerGitService(mux *http.ServeMux) {
	// Get commit
	// GET /repos/octocat/example/git/commits/{sha}
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/git/commits/{sha}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		s := strings.Split(req.URL.Path, "/")
		sha := s[len(s)-1]
		if sha == "HEAD" { // Special case
			if r.headCommit == nil {
				m.notFoundResponse(req.Context(), w)
			}
			m.jsonResponse(req.Context(), w, http.StatusOK, r.headCommit.ghCommit)
		}
		for _, v := range r.commits {
			if v.ghCommit.GetSHA() == sha {
				m.jsonResponse(req.Context(), w, http.StatusOK, v.ghCommit)
			}
		}
		m.notFoundResponse(req.Context(), w)
	})
	// Get tree
	// Get /repos/octocat/example/git/trees/{sha}
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/git/trees/{sha}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		sha := req.PathValue("sha")
		var prefix *string
		for _, c := range r.commits {
			for _, v := range c.files {
				if v.sha == sha {
					prefix = new(v.Name)
					break
				}
			}
		}
		if prefix == nil {
			m.notFoundResponse(req.Context(), w)
		}

		var entries []*github.TreeEntry
		for _, c := range r.commits {
			for _, v := range c.files[1:] { // Exclude root node
				// Repository root
				if *prefix == "" {
					if strings.Index(v.Name, "/") == -1 {
						ft := "blob"
						if v.mode == fileTypeDir {
							ft = "tree"
						}
						entries = append(entries, &github.TreeEntry{
							SHA:  new(v.sha),
							Type: new(ft),
							Path: new(v.Name),
						})
					}
					continue
				}

				if strings.HasPrefix(v.Name, *prefix) && v.Name != *prefix {
					// Exclude children
					rest := v.Name[strings.Index(v.Name, *prefix)+len(*prefix)+1:]
					if strings.Index(rest, "/") != -1 {
						continue
					}

					ft := "blog"
					if v.mode == fileTypeDir {
						ft = "tree"
					}
					entries = append(entries, &github.TreeEntry{
						SHA:  new(v.sha),
						Type: new(ft),
						Path: new(strings.TrimPrefix(v.Name, *prefix+"/")),
					})
				}
			}
		}
		tree := &github.Tree{
			SHA:     new(sha),
			Entries: entries,
		}
		m.jsonResponse(req.Context(), w, http.StatusOK, tree)
	})
	// Get blob
	// GET /repos/octocat/example/git/blobs/{sha}
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/git/blobs/{sha}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		sha := req.PathValue("sha")
		for _, c := range r.commits {
			for _, v := range c.files {
				if v.sha == sha {
					w.Write(v.Body)
					return
				}
			}
		}
		m.notFoundResponse(req.Context(), w)
	})
	// Get ref
	// GET /repos/octocat/example/git/ref/tags/{sha}
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/git/ref/tags/{sha}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		s := strings.Split(req.URL.Path, "/")
		ref := plumbing.ReferenceName("refs/" + strings.Join(s[6:], "/"))
		if ref.IsTag() {
			for _, v := range r.tags {
				if v.ghTag.GetTag() == ref.Short() {
					reference := &github.Reference{
						Ref: new(ref.String()),
						Object: &github.GitObject{
							SHA:  v.commit.ghCommit.SHA,
							Type: new("commit"),
						},
					}
					m.jsonResponse(req.Context(), w, http.StatusOK, reference)
				}
			}
		}
		m.notFoundResponse(req.Context(), w)
	})
}

func (m *Mock) registerRepositoriesService(mux *http.ServeMux) {
	// Get a repository
	// GET /repos/{owner}/{repo}
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		m.jsonResponse(req.Context(), w, http.StatusOK, r.ghRepository)
	})
	// Get commit
	// GET /repos/octocat/example/commits/{sha}
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/commits/{sha}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		sha := req.PathValue("sha")
		if sha == "HEAD" { // Special case
			if r.headCommit == nil {
				m.notFoundResponse(req.Context(), w)
			}
			m.jsonResponse(req.Context(), w, http.StatusOK, &github.RepositoryCommit{SHA: r.headCommit.ghCommit.SHA, Commit: r.headCommit.ghCommit})
		}
		for _, c := range r.commits {
			if c.ghCommit.GetSHA() == sha {
				commit := &github.RepositoryCommit{
					SHA:    c.ghCommit.SHA,
					Commit: c.ghCommit,
				}
				m.jsonResponse(req.Context(), w, http.StatusOK, commit)
			}
		}
		m.notFoundResponse(req.Context(), w)
	})
	// Create commit status
	// POST /repos/octocat/example/statuses/{sha}
	m.registerHandleFunc(mux, "POST /repos/{owner}/{repo}/statuses/{sha}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		var status github.RepoStatus
		if err := json.NewDecoder(req.Body).Decode(&status); err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}

		sha := req.PathValue("sha")
		var commit *Commit
		if sha == "HEAD" {
			if r.headCommit == nil {
				m.notFoundResponse(req.Context(), w)
			}
			commit = r.headCommit
		} else {
			for _, v := range r.commits {
				if v.ghCommit.GetSHA() == sha {
					commit = v
					break
				}
			}
		}
		if commit == nil {
			m.notFoundResponse(req.Context(), w)
		}
		commit.ghStatuses = append(commit.ghStatuses, &status)
		m.jsonResponse(req.Context(), w, http.StatusOK, status)
	})
}

func (m *Mock) registerIssuesService(mux *http.ServeMux) {
	// Get an issue
	// GET /repos/octocat/example/issues/{number}
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/issues/{number}", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}

		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}
		issue := r.GetIssue(num)
		m.jsonResponse(req.Context(), w, http.StatusOK, issue.ghIssue)
	})
	// Get issues
	// GET /repos/octocat/example/issues
	m.registerHandleFunc(mux, "GET /repos/{owner}/{repo}/issues", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}

		var issues []*github.Issue
		for _, v := range r.GetIssues() {
			issues = append(issues, v.ghIssue)
		}
		m.jsonWithPaginationResponse(req.Context(), w, req, http.StatusOK, issues)
	})
	// Create issue
	// Post /repos/octocat/example/issues
	m.registerHandleFunc(mux, "POST /repos/{owner}/{repo}/issues", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}

		var reqIssue github.IssueRequest
		if err := json.NewDecoder(req.Body).Decode(&reqIssue); err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}

		issue := &github.Issue{
			Number: new(r.NextIndex()),
			Title:  reqIssue.Title,
			Body:   reqIssue.Body,
		}
		r.Issues(&Issue{ghIssue: issue})
		m.jsonResponse(req.Context(), w, http.StatusOK, issue)
	})
	// Create a new comment
	// POST /repos/octocat/example/issues/1/comments
	m.registerHandleFunc(mux, "POST /repos/{owner}/{repo}/issues/{number}/comments", func(w http.ResponseWriter, req *http.Request) {
		r := m.findRepository(req)
		if r == nil {
			m.notFoundResponse(req.Context(), w)
		}
		num, err := strconv.Atoi(req.PathValue("number"))
		if err != nil {
			m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
		}

		issue := r.GetIssue(num)
		if issue != nil {
			var comment github.IssueComment
			if err := json.NewDecoder(req.Body).Decode(&comment); err != nil {
				m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
			}

			issue.comments = append(issue.comments, &comment)
			m.jsonResponse(req.Context(), w, http.StatusOK, comment)
		}

		pr := r.GetPullRequest(num)
		if pr != nil {
			var comment github.IssueComment
			if err := json.NewDecoder(req.Body).Decode(&comment); err != nil {
				m.errResponse(req.Context(), w, http.StatusBadRequest, err.Error())
			}

			pr.comments = append(pr.comments, &github.PullRequestComment{
				Body: comment.Body,
			})
			m.jsonResponse(req.Context(), w, http.StatusOK, comment)
		}

		m.notFoundResponse(req.Context(), w)
	})
}

func (m *Mock) registerUsersService(mux *http.ServeMux) {
	// Get a user
	// GET /users/{username}
	m.registerHandleFunc(mux, "GET /users/{username}", func(w http.ResponseWriter, req *http.Request) {
		u := m.findUser(req)
		if u == nil {
			m.notFoundResponse(req.Context(), w)
		}
		m.jsonResponse(req.Context(), w, http.StatusOK, u.ghUser)
	})
}

func (m *Mock) registerTeamsService(mux *http.ServeMux) {
	// Get a team
	// GET /orgs/{org}/teams/{team_slug}
	m.registerHandleFunc(mux, "GET /orgs/{org}/teams/{team_slug}", func(w http.ResponseWriter, req *http.Request) {
		team := m.findTeam(req)
		if team == nil {
			m.notFoundResponse(req.Context(), w)
		}
		m.jsonResponse(req.Context(), w, http.StatusOK, team.ghTeam)
	})
	// Get team members by slug
	// GET /orgs/{org}/teams/{team_slug}/members
	m.registerHandleFunc(mux, "GET /orgs/{org}/teams/{team_slug}/members", func(w http.ResponseWriter, req *http.Request) {
		team := m.findTeam(req)
		if team == nil {
			m.notFoundResponse(req.Context(), w)
		}
		var users []*github.User
		for _, u := range m.users {
			if _, ok := u.teams[fmt.Sprintf("%s/%s", *team.ghTeam.Organization.Login, *team.ghTeam.Slug)]; ok {
				users = append(users, u.ghUser)
			}
		}
		m.jsonResponse(req.Context(), w, http.StatusOK, users)
	})
}

func (m *Mock) findRepository(req *http.Request) *Repository {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.repositories[fmt.Sprintf("%s/%s", req.PathValue("owner"), req.PathValue("repo"))]; ok {
		return r
	}
	return nil
}

func (m *Mock) findUser(req *http.Request) *User {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[req.PathValue("username")]; ok {
		return u
	}
	return nil
}

func (m *Mock) findTeam(req *http.Request) *Team {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.teams[fmt.Sprintf("%s/%s", req.PathValue("org"), req.PathValue("team_slug"))]; ok {
		return t
	}
	return nil
}

var escape = errors.New("escape")

func (m *Mock) notFoundResponse(ctx context.Context, w http.ResponseWriter) {
	m.errResponse(ctx, w, http.StatusNotFound, "Not found")
}

func (m *Mock) noContentResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
	panic(escape)
}

type errorResponse struct {
	Message string `json:"message"`
}

func (m *Mock) errResponse(ctx context.Context, w http.ResponseWriter, status int, message string) {
	m.jsonResponse(ctx, w, status, &errorResponse{Message: message})
}

func (m *Mock) jsonResponse(ctx context.Context, w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		m.Logger.ErrorContext(ctx, "failed to encode response", slog.Any("err", err))
	}
	panic(escape)
}

func (m *Mock) jsonWithPaginationResponse(ctx context.Context, w http.ResponseWriter, req *http.Request, status int, data any) {
	val := reflect.ValueOf(data)
	if val.Kind() != reflect.Slice {
		panic("data must be a slice")
	}

	perPage, err := strconv.Atoi(req.URL.Query().Get("per_page"))
	if perPage <= 0 || err != nil {
		perPage = 30
	}
	page, err := strconv.Atoi(req.URL.Query().Get("page"))
	if page <= 0 || err != nil {
		page = 1
	}
	first := (page - 1) * perPage
	end := first + perPage
	if val.Len() < end {
		end = val.Len()
	}
	lastPage := val.Len()/perPage + 1
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	linkURL := &url.URL{}
	*linkURL = *req.URL
	linkURL.Scheme = m.Scheme
	if (m.Scheme == "http" && m.Port == 80) || (m.Scheme == "https" && m.Port == 443) {
		linkURL.Host = m.Host
	} else {
		linkURL.Host = fmt.Sprintf("%s:%d", m.Host, m.Port)
	}
	linkURL.Host = "localhost:5620"
	var links []string
	if page > 1 {
		prevLink := *linkURL
		q := prevLink.Query()
		q.Set("page", strconv.Itoa(page-1))
		prevLink.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf(`<%s>; rel="prev"`, prevLink.String()))
		firstLink := *req.URL
		q = firstLink.Query()
		q.Set("page", strconv.Itoa(1))
		firstLink.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf(`<%s>; rel="first"`, firstLink.String()))
	}
	if page != lastPage {
		lastLink := *linkURL
		q := lastLink.Query()
		q.Set("page", fmt.Sprintf("%d", lastPage))
		lastLink.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf(`<%s>; rel="last"`, lastLink.String()))
	}
	if page < lastPage {
		nextLink := *linkURL
		q := nextLink.Query()
		q.Set("page", strconv.Itoa(page+1))
		nextLink.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf(`<%s>; rel="next"`, nextLink.String()))
	}
	w.Header().Set("Link", strings.Join(links, ", "))
	w.WriteHeader(status)
	ret := val.Slice(first, end)
	if err := json.NewEncoder(w).Encode(ret.Interface()); err != nil {
		m.Logger.ErrorContext(ctx, "failed to encode response", slog.Any("err", err))
	}
}

func newHash() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	h := sha256.New()
	hash := h.Sum(buf)
	return hex.EncodeToString(hash)
}

type transport struct {
	handler http.Handler
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	recoder := httptest.NewRecorder()
	t.handler.ServeHTTP(recoder, req)
	return recoder.Result(), nil
}

func firstParentSHA(c *Commit) string {
	for _, p := range c.ghCommit.Parents {
		return p.GetSHA()
	}
	return ""
}
