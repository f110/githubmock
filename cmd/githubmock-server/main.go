package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.f110.dev/githubmock"
	"go.f110.dev/githubmock/cmd/githubmock-server/internal/config"
)

var (
	listen = flag.String("listen", ":5620", "Listen address")
)

func main() {
	flag.Parse()

	teams, users, repos, err := config.Load(flag.Args()...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	mock, err := newMock(teams, users, repos)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create mock: %v\n", err)
		os.Exit(1)
	}

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
	mock.Scheme = "http"
	mock.Host = "localhost"
	mock.Port = port

	mux := http.NewServeMux()
	mock.RegisterHandler(mux)
	svr := &http.Server{
		Addr:    *listen,
		Handler: accessLogWrapper(mux),
	}
	fmt.Printf("Listening on %s\n", *listen)
	if err := svr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func newMock(teams []*config.Team, users []*config.User, repos []*config.Repository) (*githubmock.Mock, error) {
	mock := githubmock.NewMock()
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
				b.Head(m[pr.Head.Repo], pr.Head.Ref)
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
