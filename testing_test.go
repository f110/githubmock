package githubmock

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v83/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMock(t *testing.T) {
	t.Run("PullRequestService", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("f110/gh-test")
		repo.PullRequests(
			NewPullRequest().
				Number(1).
				Title(t.Name()).
				State(PullRequestStateOpen).
				Body("PR description").
				Base("master").
				Head(nil, "feature-1").
				Mergeable().
				CreatedAt(time.Date(2022, 1, 1, 1, 1, 1, 0, time.UTC)).
				UpdatedAt(time.Date(2022, 1, 1, 1, 1, 1, 0, time.UTC)),
			NewPullRequest().
				Number(2).
				Title("PR 2").
				State(PullRequestStateClosed).
				Base("master").
				Reviews(
					NewReview().
						Author("user1").
						Body("LGTM").
						State(ReviewStateApproved),
				).
				CreatedAt(time.Date(2023, 1, 1, 1, 1, 1, 0, time.UTC)).
				UpdatedAt(time.Date(2023, 1, 1, 1, 1, 1, 0, time.UTC)),
		)

		ghClient := github.NewClient(&http.Client{Transport: m.Transport()})

		t.Run("Get", func(t *testing.T) {
			pr, _, err := ghClient.PullRequests.Get(t.Context(), "f110", "gh-test", 1)
			require.NoError(t, err)
			assert.Equal(t, 1, pr.GetNumber())
			require.NotNil(t, pr.Base)
			require.NotNil(t, pr.Base.Repo)
			assert.Equal(t, "f110/gh-test", pr.Base.Repo.GetFullName())
			assert.Equal(t, "2022-01-01T01:01:01Z", pr.GetCreatedAt().Format(time.RFC3339))
			assert.Equal(t, "2022-01-01T01:01:01Z", pr.GetUpdatedAt().Format(time.RFC3339))
			assert.True(t, pr.GetMergeable())
		})

		t.Run("List", func(t *testing.T) {
			prs, _, err := ghClient.PullRequests.List(t.Context(), "f110", "gh-test", &github.PullRequestListOptions{State: "open"})
			require.NoError(t, err)
			assert.Len(t, prs, 1)
			assert.NotNil(t, prs[0].CreatedAt)
			assert.NotNil(t, prs[0].UpdatedAt)

			prs, _, err = ghClient.PullRequests.List(t.Context(), "f110", "gh-test", &github.PullRequestListOptions{State: "closed"})
			require.NoError(t, err)
			assert.Len(t, prs, 1)
			assert.Equal(t, "closed", prs[0].GetState())

			prs, _, err = ghClient.PullRequests.List(t.Context(), "f110", "gh-test", &github.PullRequestListOptions{State: "all"})
			require.NoError(t, err)
			assert.Len(t, prs, 2)

			prs, _, err = ghClient.PullRequests.List(t.Context(), "f110", "gh-test", &github.PullRequestListOptions{State: "all", Sort: "created"})
			require.NoError(t, err)
			assert.Len(t, prs, 2)
			assert.Equal(t, 2, prs[0].GetNumber())

			prs, _, err = ghClient.PullRequests.List(t.Context(), "f110", "gh-test", &github.PullRequestListOptions{State: "all", Sort: "updated", Direction: "asc"})
			require.NoError(t, err)
			assert.Len(t, prs, 2)
			assert.Equal(t, 1, prs[0].GetNumber())
		})

		t.Run("Create", func(t *testing.T) {
			pr, _, err := ghClient.PullRequests.Create(t.Context(), "f110", "gh-test", &github.NewPullRequest{})
			require.NoError(t, err)
			assert.Equal(t, 3, pr.GetNumber())
		})

		t.Run("Edit", func(t *testing.T) {
			repo.PullRequests(
				NewPullRequest().
					Number(4).
					Title(t.Name()).
					Body("PR description").
					Base("master").
					Head(nil, "feature-1"),
			)

			pr, _, err := ghClient.PullRequests.Edit(t.Context(), "f110", "gh-test", 4, &github.PullRequest{
				Base: &github.PullRequestBranch{Ref: new("main")},
			})
			require.NoError(t, err)
			assert.Equal(t, t.Name(), pr.GetTitle())
			assert.Equal(t, "main", pr.GetBase().GetRef())
		})

		t.Run("CreateComment", func(t *testing.T) {
			comment, _, err := ghClient.PullRequests.CreateComment(t.Context(), "f110", "gh-test", 1, &github.PullRequestComment{
				Body: new("Comment"),
			})
			require.NoError(t, err)
			assert.NotNil(t, comment)
			pr := repo.GetPullRequest(1)
			require.NotNil(t, pr)
			assert.Len(t, pr.comments, 1)
		})

		t.Run("ListReviews", func(t *testing.T) {
			reviews, _, err := ghClient.PullRequests.ListReviews(t.Context(), "f110", "gh-test", 2, nil)
			require.NoError(t, err)
			assert.Len(t, reviews, 1)
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

		ghClient := github.NewClient(&http.Client{Transport: m.Transport()})

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
		repo.DefaultBranch("main")
		err := repo.Commits(
			NewCommit().
				IsHead().
				Files(&File{Name: "README.md", Body: []byte("README")}),
		)
		require.NoError(t, err)

		ghClient := github.NewClient(&http.Client{Transport: m.Transport()})

		t.Run("Get", func(t *testing.T) {
			r, _, err := ghClient.Repositories.Get(t.Context(), "f110", "gh-test")
			require.NoError(t, err)
			assert.Equal(t, "gh-test", r.GetName())
			assert.Equal(t, "f110/gh-test", r.GetFullName())
			assert.Equal(t, "main", r.GetDefaultBranch())
		})

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

	t.Run("IssuesService", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("f110/gh-test")
		repo.Issues(
			NewIssue().
				Number(1).
				Title(t.Name()).
				CreatedAt(time.Date(2022, 1, 1, 1, 1, 1, 0, time.UTC)).
				UpdatedAt(time.Date(2022, 1, 1, 1, 1, 1, 0, time.UTC)),
		)

		ghClient := github.NewClient(&http.Client{Transport: m.Transport()})

		t.Run("Get", func(t *testing.T) {
			issue, _, err := ghClient.Issues.Get(t.Context(), "f110", "gh-test", 1)
			require.NoError(t, err)
			assert.Equal(t, 1, issue.GetNumber())
			require.NotNil(t, issue.Repository)
			assert.Equal(t, "2022-01-01T01:01:01Z", issue.GetCreatedAt().Format(time.RFC3339))
			assert.Equal(t, "2022-01-01T01:01:01Z", issue.GetUpdatedAt().Format(time.RFC3339))
		})

		t.Run("ListByRepo", func(t *testing.T) {
			issues, _, err := ghClient.Issues.ListByRepo(t.Context(), "f110", "gh-test", nil)
			require.NoError(t, err)
			assert.Len(t, issues, 1)
		})

		t.Run("Create", func(t *testing.T) {
			pr, _, err := ghClient.Issues.Create(t.Context(), "f110", "gh-test", &github.IssueRequest{})
			require.NoError(t, err)
			assert.Equal(t, 2, pr.GetNumber())
		})

		t.Run("CreateComment", func(t *testing.T) {
			comment, _, err := ghClient.Issues.CreateComment(t.Context(), "f110", "gh-test", 1, &github.IssueComment{
				Body: new("Comment"),
			})
			require.NoError(t, err)
			assert.NotNil(t, comment)
			issue := repo.GetIssue(1)
			require.NotNil(t, issue)
			assert.Len(t, issue.comments, 1)
		})
	})

	t.Run("UsersService", func(t *testing.T) {
		m := NewMock()
		m.Repository("f110/gh-test")
		m.User("octocat")

		ghClient := github.NewClient(&http.Client{Transport: m.Transport()})

		t.Run("Get", func(t *testing.T) {
			user, _, err := ghClient.Users.Get(t.Context(), "octocat")
			require.NoError(t, err)
			assert.Equal(t, "octocat", user.GetLogin())

			user, _, err = ghClient.Users.Get(t.Context(), "f110")
			require.NoError(t, err)
			assert.Equal(t, "f110", user.GetLogin())
		})
	})

	t.Run("TeamsService", func(t *testing.T) {
		m := NewMock()
		m.Team("f110/team1")
		m.User("octocat").Team("f110/team1")
		m.User("octocat2").Team("f110/team1")

		ghClient := github.NewClient(&http.Client{Transport: m.Transport()})

		t.Run("GetTeamBySlug", func(t *testing.T) {
			team, _, err := ghClient.Teams.GetTeamBySlug(t.Context(), "f110", "team1")
			require.NoError(t, err)
			assert.Equal(t, "team1", team.GetSlug())
		})

		t.Run("ListTeamMembersBySlug", func(t *testing.T) {
			users, _, err := ghClient.Teams.ListTeamMembersBySlug(t.Context(), "f110", "team1", nil)
			require.NoError(t, err)
			require.Len(t, users, 2)
		})
	})
}
