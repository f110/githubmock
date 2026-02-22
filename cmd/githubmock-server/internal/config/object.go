package config

import (
	"errors"
	"io"
	"os"

	"go.yaml.in/yaml/v4"

	"go.f110.dev/githubmock"
)

type Repository struct {
	Name         string         `yaml:"name"`
	PullRequests []*PullRequest `yaml:"pull_requests,omitempty"`
	Issues       []*Issue       `yaml:"issues,omitempty"`
	Tags         []*Tag         `yaml:"tags,omitempty"`
	Commits      []*Commit      `yaml:"commits,omitempty"`
}

type PullRequest struct {
	Number   int        `yaml:"number,omitempty"`
	Title    string     `yaml:"title,omitempty"`
	Body     string     `yaml:"body,omitempty"`
	Base     string     `yaml:"base,omitempty"`
	Head     *Head      `yaml:"head,omitempty"`
	Comments []*Comment `yaml:"comments,omitempty"`
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
	Number   int        `yaml:"number,omitempty"`
	Title    string     `yaml:"title,omitempty"`
	Comments []*Comment `yaml:"comments,omitempty"`
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
	State       githubmock.CommitState `yaml:"state"`
	Description string                 `yaml:"description,omitempty"`
}

type File struct {
	Name    string `yaml:"name"`
	Content string `yaml:"content"`
}

func Load(definitionFiles ...string) ([]*Repository, error) {
	var repos []*Repository
	for _, v := range definitionFiles {
		f, err := os.Open(v)
		if err != nil {
			return nil, err
		}

		decoder := yaml.NewDecoder(f)
		for {
			repo := &Repository{}
			err := decoder.Decode(repo)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			repos = append(repos, repo)
		}
	}
	return repos, nil
}
