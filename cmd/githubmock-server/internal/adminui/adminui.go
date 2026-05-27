package adminui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"go.f110.dev/githubmock"
)

//go:embed static/*
var staticFiles embed.FS

func Register(mux *http.ServeMux, mock *githubmock.Mock) {
	h := &handler{mock: mock}

	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /_admin/", http.StripPrefix("/_admin/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /_admin/api/repositories", h.listRepositories)
	mux.HandleFunc("POST /_admin/api/send", h.send)
}

type handler struct {
	mock *githubmock.Mock
}

type webhookView struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type repoView struct {
	FullName     string        `json:"full_name"`
	PullRequests []prView      `json:"pull_requests"`
	Commits      []shaView     `json:"commits"`
	Webhooks     []webhookView `json:"webhooks"`
}

type prView struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

type shaView struct {
	SHA string `json:"sha"`
}

func (h *handler) listRepositories(w http.ResponseWriter, _ *http.Request) {
	repos := h.mock.Repositories()
	out := make([]repoView, 0, len(repos))
	for _, r := range repos {
		view := repoView{FullName: r.FullName()}
		for _, pr := range r.GetPullRequests() {
			view.PullRequests = append(view.PullRequests, prView{
				Number: pr.GetNumber(),
				Title:  pr.GetTitle(),
				State:  pr.GetState(),
			})
		}
		for _, c := range r.GetCommits() {
			view.Commits = append(view.Commits, shaView{SHA: c.GetSHA()})
		}
		for _, hook := range r.Webhooks() {
			view.Webhooks = append(view.Webhooks, webhookView{URL: hook.GetURL(), Events: hook.GetEvents()})
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, out)
}

type sendRequest struct {
	Event      string `json:"event"`
	Action     string `json:"action"`
	Repository string `json:"repository"`
	Number     int    `json:"number"`
	Ref        string `json:"ref"`
	SHA        string `json:"sha"`
	Sender     string `json:"sender"`
}

type deliveryView struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

func (h *handler) send(w http.ResponseWriter, req *http.Request) {
	var body sendRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	repo := h.mock.LookupRepository(body.Repository)
	if repo == nil {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("repository %q not found", body.Repository))
		return
	}

	var sender *githubmock.User
	if body.Sender != "" {
		sender = h.mock.User(body.Sender)
	}

	var payload any
	switch body.Event {
	case "pull_request":
		pr := repo.GetPullRequest(body.Number)
		if pr == nil {
			writeErr(w, http.StatusNotFound, fmt.Sprintf("pull request #%d not found", body.Number))
			return
		}
		action := body.Action
		if action == "" {
			action = "opened"
		}
		payload = repo.BuildPullRequestEvent(action, pr, sender)
	case "push":
		commit := repo.GetCommit(body.SHA)
		if commit == nil {
			writeErr(w, http.StatusNotFound, fmt.Sprintf("commit %q not found", body.SHA))
			return
		}
		ref := body.Ref
		if ref == "" {
			ref = "refs/heads/main"
		}
		payload = repo.BuildPushEvent(ref, commit, sender)
	default:
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("unsupported event %q", body.Event))
		return
	}

	deliveries := repo.SendWebhook(req.Context(), body.Event, payload)
	views := make([]deliveryView, 0, len(deliveries))
	for _, d := range deliveries {
		v := deliveryView{URL: d.URL, StatusCode: d.StatusCode}
		if d.Err != nil {
			v.Error = d.Err.Error()
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event":      body.Event,
		"payload":    payload,
		"deliveries": views,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeErr(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
