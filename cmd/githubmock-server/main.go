package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"

	"go.f110.dev/githubmock"
	"go.f110.dev/githubmock/cmd/githubmock-server/internal/config"
)

var (
	listen = flag.String("listen", ":5620", "Listen address")
)

func main() {
	flag.Parse()

	repos, err := config.Load(flag.Args()...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	mock, err := newMock(repos)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create mock: %v\n", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mock.RegisterHandler(mux)
	svr := &http.Server{
		Addr:    *listen,
		Handler: mux,
	}
	fmt.Printf("Listening on %s\n", *listen)
	if err := svr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func newMock(repos []*config.Repository) (*githubmock.Mock, error) {
	mock := githubmock.NewMock()
	m := make(map[string]*githubmock.Repository)
	for _, v := range repos {
		m[v.Name] = mock.Repository(v.Name)
	}

	for _, confRepo := range repos {
		repo := m[confRepo.Name]

		for _, pr := range confRepo.PullRequests {
			comments := make([]*githubmock.Comment, 0, len(pr.Comments))
			for _, c := range pr.Comments {
				comments = append(comments, &githubmock.Comment{Author: c.Author, Body: c.Body})
			}
			b := githubmock.NewPullRequest().
				Number(pr.Number).
				Title(pr.Title).
				Body(pr.Body).
				Base(pr.Base).
				Comments(comments)
			if pr.Head != nil {
				b.Head(m[pr.Head.Repo], pr.Head.Ref)
			}
			repo.PullRequests(b)
		}

		for _, issue := range confRepo.Issues {
			comments := make([]*githubmock.Comment, 0, len(issue.Comments))
			for _, c := range issue.Comments {
				comments = append(comments, &githubmock.Comment{Author: c.Author, Body: c.Body})
			}
			b := githubmock.NewIssue().
				Number(issue.Number).
				Title(issue.Title).
				Comments(comments)
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
