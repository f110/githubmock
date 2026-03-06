package config

import (
	"errors"
	"io"
	"os"
	"time"

	"go.yaml.in/yaml/v4"

	"go.f110.dev/githubmock"
)

type (
	PullRequestState = githubmock.PullRequestState
	IssueState       = githubmock.IssueState
	ReviewState      = githubmock.ReviewState
	CommitState      = githubmock.CommitState
)

type Repository struct {
	Name         string         `yaml:"name"`
	PullRequests []*PullRequest `yaml:"pull_requests,omitempty"`
	Issues       []*Issue       `yaml:"issues,omitempty"`
	Tags         []*Tag         `yaml:"tags,omitempty"`
	Commits      []*Commit      `yaml:"commits,omitempty"`
}

type PullRequest struct {
	Number    int              `yaml:"number,omitempty"`
	Title     string           `yaml:"title,omitempty"`
	Author    string           `yaml:"author,omitempty"`
	Body      string           `yaml:"body,omitempty"`
	Base      string           `yaml:"base,omitempty"`
	Head      *Head            `yaml:"head,omitempty"`
	State     PullRequestState `yaml:"state,omitempty"`
	Comments  []*Comment       `yaml:"comments,omitempty"`
	Reviews   []*Review        `yaml:"reviews,omitempty"`
	CreatedAt time.Time        `yaml:"created_at,omitempty"`
	UpdatedAt time.Time        `yaml:"updated_at,omitempty"`
}

type Head struct {
	Repo string `yaml:"repo"`
	Ref  string `yaml:"ref"`
}

type Comment struct {
	Author string `yaml:"author"`
	Body   string `yaml:"body"`
}

type Issue struct {
	Number    int        `yaml:"number,omitempty"`
	Title     string     `yaml:"title,omitempty"`
	Author    string     `yaml:"author,omitempty"`
	State     IssueState `yaml:"state,omitempty"`
	Comments  []*Comment `yaml:"comments,omitempty"`
	CreatedAt time.Time  `yaml:"created_at,omitempty"`
	UpdatedAt time.Time  `yaml:"updated_at,omitempty"`
}

type Tag struct {
	Name   string `yaml:"name"`
	Commit string `yaml:"commit,omitempty"`
}

type Commit struct {
	SHA      string          `yaml:"sha"`
	Parents  []string        `yaml:"parents,omitempty"`
	Files    []*File         `yaml:"files,omitempty"`
	Statuses []*CommitStatus `yaml:"statuses,omitempty"`
}

type CommitStatus struct {
	State       CommitState `yaml:"state"`
	Description string      `yaml:"description,omitempty"`
}

type File struct {
	Name    string `yaml:"name"`
	Content string `yaml:"content"`
}

type Review struct {
	State  ReviewState `yaml:"state"`
	Author string      `yaml:"author,omitempty"`
	Body   string      `yaml:"body,omitempty"`
}

func Load(definitionFiles ...string) ([]*Repository, error) {
	var repos []*Repository
	for _, v := range definitionFiles {
		f, err := os.Open(v)
		if err != nil {
			return nil, err
		}

		decoder := yaml.NewDecoder(f)
		var node yaml.Node
		for {
			err := decoder.Decode(&node)
			if errors.Is(err, io.EOF) {
				break
			}
			var k struct {
				Kind string `yaml:"kind"`
			}
			if err := node.Decode(&k); err != nil {
				return nil, err
			}

			switch k.Kind {
			default:
				repo := &Repository{}
				if err != nil {
					return nil, err
				}
				if err := node.Decode(repo); err != nil {
					return nil, err
				}
				repos = append(repos, repo)
			}
		}
	}
	return repos, nil
}
