package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"go.f110.dev/githubmock"
	"go.f110.dev/githubmock/cmd/githubmock-server/internal/adminui"
	"go.f110.dev/githubmock/cmd/githubmock-server/internal/config"
)

var (
	listen    = flag.String("listen", ":5620", "Listen address")
	githubURL = flag.String("github-url", "https://github.com", "Base URL used for repository.html_url in webhook payloads")
	watch     = flag.Bool("watch", false, "Watch the given configuration files and reload on change")
)

func main() {
	flag.Parse()
	files := flag.Args()

	_, p, err := net.SplitHostPort(*listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse listen address: %v\n", err)
		os.Exit(1)
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse port: %v\n", err)
		os.Exit(1)
	}

	build := func() (http.Handler, error) {
		teams, users, repos, err := config.Load(files...)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
		mock, err := newMock(teams, users, repos, *githubURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create mock: %w", err)
		}
		mock.Scheme = "http"
		mock.Host = "localhost"
		mock.Port = port

		mux := http.NewServeMux()
		mock.RegisterHandler(mux)
		adminui.Register(mux, mock)
		return mux, nil
	}

	handler, err := build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	rh := &reloadableHandler{}
	rh.Store(handler)

	if *watch && len(files) > 0 {
		go watchFiles(context.Background(), files, 1*time.Second, func() {
			h, err := build()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to reload config: %v\n", err)
				return
			}
			rh.Store(h)
			fmt.Fprintln(os.Stdout, "Configuration reloaded")
		})
	}

	svr := &http.Server{
		Addr:    *listen,
		Handler: accessLogWrapper(rh),
	}
	fmt.Printf("Listening on %s\n", *listen)
	fmt.Printf("Admin UI: http://localhost:%d/_admin/\n", port)
	if err := svr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

type reloadableHandler struct {
	h atomic.Pointer[http.Handler]
}

func (rh *reloadableHandler) Store(h http.Handler) {
	rh.h.Store(&h)
}

func (rh *reloadableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	(*rh.h.Load()).ServeHTTP(w, r)
}

func watchFiles(ctx context.Context, files []string, interval time.Duration, onChange func()) {
	mtimes := make(map[string]time.Time, len(files))
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			mtimes[f] = fi.ModTime()
		}
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			changed := false
			for _, f := range files {
				fi, err := os.Stat(f)
				if err != nil {
					continue
				}
				if prev, ok := mtimes[f]; !ok || !prev.Equal(fi.ModTime()) {
					mtimes[f] = fi.ModTime()
					changed = true
				}
			}
			if changed {
				onChange()
			}
		}
	}
}

func newMock(teams []*config.Team, users []*config.User, repos []*config.Repository, githubURL string) (*githubmock.Mock, error) {
	mock := githubmock.NewMock()
	if githubURL != "" {
		mock.GitHubURL = githubURL
	}
	for _, t := range teams {
		mock.
			Team(fmt.Sprintf("%s/%s", t.Organization, t.Slug)).
			Name(t.Name)
	}
	for _, u := range users {
		user := mock.User(u.Login)
		for _, v := range u.Teams {
			user.Team(v)
		}
		user.Name(u.Name).AvatarURL(u.AvatarURL)
	}

	m := make(map[string]*githubmock.Repository)
	for _, v := range repos {
		m[v.Name] = mock.Repository(v.Name)
		if v.DefaultBranch != "" {
			m[v.Name].DefaultBranch(v.DefaultBranch)
		}
	}

	for _, confRepo := range repos {
		repo := m[confRepo.Name]

		for _, w := range confRepo.Webhooks {
			repo.Webhook(githubmock.NewWebhook().URL(w.URL).Secret(w.Secret).Events(w.Events...))
		}

		for _, pr := range confRepo.PullRequests {
			comments := make([]*githubmock.PullRequestComment, 0, len(pr.Comments))
			for _, c := range pr.Comments {
				comments = append(comments, githubmock.NewPullRequestComment().Author(mock.User(c.Author)).Body(c.Body))
			}
			reviews := make([]*githubmock.Review, 0, len(pr.Reviews))
			for _, r := range pr.Reviews {
				reviews = append(reviews, githubmock.NewReview().Author(r.Author).State(r.State).Body(r.Body))
			}
			b := githubmock.NewPullRequest().
				Number(pr.Number).
				Title(pr.Title).
				State(pr.State).
				Author(mock.User(pr.Author)).
				Body(pr.Body).
				Base(pr.Base).
				Comments(comments...).
				Reviews(reviews...).
				CreatedAt(pr.CreatedAt).
				UpdatedAt(pr.UpdatedAt)
			if pr.Head != nil {
				b.Head(m[pr.Head.Repo], pr.Head.Ref, pr.Head.SHA)
			}
			if pr.Mergeable {
				b.Mergeable()
			}
			if pr.Merged {
				b.Merged()
			}
			repo.PullRequests(b)
		}

		for _, issue := range confRepo.Issues {
			comments := make([]*githubmock.Comment, 0, len(issue.Comments))
			for _, c := range issue.Comments {
				comments = append(comments, githubmock.NewComment().Author(mock.User(c.Author)).Body(c.Body))
			}
			b := githubmock.NewIssue().
				Number(issue.Number).
				Title(issue.Title).
				Author(mock.User(issue.Author)).
				State(issue.State).
				Comments(comments).
				CreatedAt(issue.CreatedAt).
				UpdatedAt(issue.UpdatedAt)
			repo.Issues(b)
		}

		commits := make(map[string]*githubmock.Commit)
		for _, commit := range confRepo.Commits {
			var files []*githubmock.File
			for _, file := range commit.Files {
				files = append(files, &githubmock.File{Name: file.Name, Body: []byte(file.Content)})
			}
			var statuses []*githubmock.CommitStatus
			for _, status := range commit.Statuses {
				statuses = append(statuses, &githubmock.CommitStatus{State: status.State, Description: status.Description})
			}
			c := githubmock.NewCommit().
				SHA(commit.SHA).
				Files(files...).
				Statuses(statuses...)
			commits[commit.SHA] = c
		}
		// Resolve parents and add commit to the mock
		for _, confCommit := range confRepo.Commits {
			commit := commits[confCommit.SHA]

			var parents []*githubmock.Commit
			for _, v := range confCommit.Parents {
				if _, ok := commits[v]; !ok {
					return nil, fmt.Errorf("parent commit %s not found", v)
				}
				parents = append(parents, commits[v])
			}
			commit.Parents(parents...)

			if err := repo.Commits(commit); err != nil {
				return nil, err
			}
		}

		for _, tag := range confRepo.Tags {
			refCommit := commits[tag.Commit]
			t := githubmock.NewTag().Name(tag.Name).Commit(refCommit)
			repo.Tags(t)
		}
	}
	return mock, nil
}

func accessLogWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t1 := time.Now()
		rr := &responseRecoder{ResponseWriter: w}
		h.ServeHTTP(rr, req)
		code := rr.code
		if code == 0 {
			code = 200
		}
		fmt.Fprintf(os.Stdout, "%s - [%s] \"%s %s %s\" %d\n", req.RemoteAddr, t1.Format("02/Jan/2006:15:04:05 -0700"), req.Method, req.URL.Path, req.Proto, code)
	})
}

type responseRecoder struct {
	http.ResponseWriter

	code int
}

func (rr *responseRecoder) WriteHeader(code int) {
	rr.code = code
	rr.ResponseWriter.WriteHeader(code)
}
