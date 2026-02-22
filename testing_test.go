package githubmock

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-github/v83/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMock(t *testing.T) {
	t.Run("PullRequestService", func(t *testing.T) {
		t.Run("Get", func(t *testing.T) {
			m := NewMock()
			repo := m.Repository("f110/gh-test")
			repo.PullRequests(
				NewPullRequest().
					Number(1).
					Title(t.Name()).
					Body("PR description").
					Base("master").
					Head(nil, "feature-1"),
			)
			ghClient := github.NewClient(&http.Client{Transport: m.RegisteredTransport()})

			pr, _, err := ghClient.PullRequests.Get(t.Context(), "f110", "gh-test", 1)
			require.NoError(t, err)
			assert.Equal(t, 1, pr.GetNumber())
		})

		t.Run("Create", func(t *testing.T) {
			m := NewMock()
			m.Repository("f110/gh-test")
			ghClient := github.NewClient(&http.Client{Transport: m.RegisteredTransport()})

			pr, _, err := ghClient.PullRequests.Create(t.Context(), "f110", "gh-test", &github.NewPullRequest{})
			require.NoError(t, err)
			assert.Equal(t, 1, pr.GetNumber())
		})

		t.Run("Edit", func(t *testing.T) {
			m := NewMock()
			repo := m.Repository("f110/gh-test")
			ghClient := github.NewClient(&http.Client{Transport: m.RegisteredTransport()})
			repo.PullRequests(
				NewPullRequest().
					Number(1).
					Title(t.Name()).
					Body("PR description").
					Base("master").
					Head(nil, "feature-1"),
			)

			pr, _, err := ghClient.PullRequests.Edit(t.Context(), "f110", "gh-test", 1, &github.PullRequest{
				Base: &github.PullRequestBranch{Ref: new("main")},
			})
			require.NoError(t, err)
			assert.Equal(t, t.Name(), pr.GetTitle())
			assert.Equal(t, "main", pr.GetBase().GetRef())
		})

		t.Run("CreateComment", func(t *testing.T) {
			m := NewMock()
			repo := m.Repository("f110/gh-test")
			ghClient := github.NewClient(&http.Client{Transport: m.RegisteredTransport()})
			repo.PullRequests(
				NewPullRequest().
					Number(1).
					Title(t.Name()),
			)

			comment, _, err := ghClient.PullRequests.CreateComment(context.Background(), "f110", "gh-test", 1, &github.PullRequestComment{
				Body: new("Comment"),
			})
			require.NoError(t, err)
			assert.NotNil(t, comment)
			pr := repo.GetPullRequest(1)
			require.NotNil(t, pr)
			assert.Len(t, pr.Comments, 1)
		})
	})

	t.Run("GitService", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("f110/gh-test")
		commit := NewCommit().
			IsHead().
			Files(
				&File{Name: ".github/CODEOWNERS"},
				&File{Name: "/docs/sample/README.md"},
				&File{Name: ".build/mirror.cue"},
				&File{Name: ".build/test.cue"},
				&File{Name: "README.md", Body: []byte("README")},
			)
		err := repo.Commits(commit)
		require.NoError(t, err)
		repo.Tags(NewTag().Name("v1.0.0").Commit(commit))

		ghClient := github.NewClient(&http.Client{Transport: m.RegisteredTransport()})

		t.Run("GetCommit", func(t *testing.T) {
			commit, _, err := ghClient.Git.GetCommit(t.Context(), "f110", "gh-test", "HEAD")
			require.NoError(t, err)
			assert.NotEmpty(t, commit.GetTree().GetSHA())
		})

		t.Run("GetTree", func(t *testing.T) {
			commit, _, err := ghClient.Git.GetCommit(t.Context(), "f110", "gh-test", "HEAD")
			require.NoError(t, err)
			require.NotEmpty(t, commit.GetTree().GetSHA())

			tree, _, err := ghClient.Git.GetTree(t.Context(), "f110", "gh-test", commit.GetTree().GetSHA(), false)
			require.NoError(t, err)
			docsSHA := ""
			buildSHA := ""
			for _, v := range tree.Entries {
				switch *v.Path {
				case "docs":
					docsSHA = *v.SHA
				case ".build":
					buildSHA = *v.SHA
				}
			}
			require.NotEmpty(t, docsSHA)
			require.NotEmpty(t, buildSHA)

			tree, _, err = ghClient.Git.GetTree(t.Context(), "f110", "gh-test", docsSHA, false)
			require.NoError(t, err)
			assert.Len(t, tree.Entries, 1)

			tree, _, err = ghClient.Git.GetTree(t.Context(), "f110", "gh-test", buildSHA, false)
			require.NoError(t, err)
			assert.Len(t, tree.Entries, 2)
		})

		t.Run("GetBlobRaw", func(t *testing.T) {
			commit, _, err := ghClient.Git.GetCommit(t.Context(), "f110", "gh-test", "HEAD")
			require.NoError(t, err)
			tree, _, err := ghClient.Git.GetTree(t.Context(), "f110", "gh-test", commit.GetTree().GetSHA(), false)
			require.NoError(t, err)
			sha := ""
			for _, v := range tree.Entries {
				if v.GetPath() == "README.md" {
					sha = v.GetSHA()
					break
				}
			}
			require.NotEmpty(t, sha)
			blob, _, err := ghClient.Git.GetBlobRaw(t.Context(), "f110", "gh-test", sha)
			require.NoError(t, err)
			assert.Equal(t, "README", string(blob))
		})

		t.Run("GetRef", func(t *testing.T) {
			ref, _, err := ghClient.Git.GetRef(t.Context(), "f110", "gh-test", "tags/v1.0.0")
			require.NoError(t, err)
			assert.NotEmpty(t, ref.GetObject().GetSHA())
			assert.Equal(t, "commit", ref.GetObject().GetType())
		})
	})

	t.Run("RepositoriesService", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("f110/gh-test")
		err := repo.Commits(
			NewCommit().
				IsHead().
				Files(&File{Name: "README.md", Body: []byte("README")}),
		)
		require.NoError(t, err)

		ghClient := github.NewClient(&http.Client{Transport: m.RegisteredTransport()})

		t.Run("GetCommit", func(t *testing.T) {
			repoCommit, _, err := ghClient.Repositories.GetCommit(t.Context(), "f110", "gh-test", "HEAD", &github.ListOptions{})
			require.NoError(t, err)
			assert.NotEmpty(t, repoCommit.GetSHA())
			assert.NotEmpty(t, repoCommit.GetCommit().GetTree().GetSHA())
		})

		t.Run("CreateStatus", func(t *testing.T) {
			status, _, err := ghClient.Repositories.CreateStatus(t.Context(), "f110", "gh-test", "HEAD", github.RepoStatus{State: new("success")})
			require.NoError(t, err)
			assert.NotEmpty(t, *status.State)
		})
	})

	t.Run("IssueService", func(t *testing.T) {
		t.Run("Create", func(t *testing.T) {
			m := NewMock()
			m.Repository("f110/gh-test")
			ghClient := github.NewClient(&http.Client{Transport: m.RegisteredTransport()})

			pr, _, err := ghClient.Issues.Create(t.Context(), "f110", "gh-test", &github.IssueRequest{})
			require.NoError(t, err)
			assert.Equal(t, 1, pr.GetNumber())
		})

		t.Run("CreateComment", func(t *testing.T) {
			m := NewMock()
			repo := m.Repository("f110/gh-test")
			ghClient := github.NewClient(&http.Client{Transport: m.RegisteredTransport()})
			repo.Issues(
				NewIssue().
					Number(1).
					Title(t.Name()),
			)

			comment, _, err := ghClient.Issues.CreateComment(t.Context(), "f110", "gh-test", 1, &github.IssueComment{
				Body: new("Comment"),
			})
			require.NoError(t, err)
			assert.NotNil(t, comment)
			issue := repo.GetIssue(1)
			require.NotNil(t, issue)
			assert.Len(t, issue.Comments, 1)
		})
	})
}
