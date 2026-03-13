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

type User struct {
	Login     string   `yaml:"login"`
	Name      string   `yaml:"name,omitempty"`
	AvatarURL string   `yaml:"avatar_url,omitempty"`
	Teams     []string `yaml:"teams,omitempty"`
}

type Team struct {
	Organization string `yaml:"organization"`
	Slug         string `yaml:"slug"`
	Name         string `yaml:"name,omitempty"`
}

type Repository struct {
	Name          string         `yaml:"name"`
	PullRequests  []*PullRequest `yaml:"pull_requests,omitempty"`
	Issues        []*Issue       `yaml:"issues,omitempty"`
	Tags          []*Tag         `yaml:"tags,omitempty"`
	Commits       []*Commit      `yaml:"commits,omitempty"`
	DefaultBranch string         `yaml:"default_branch,omitempty"`
}

type PullRequest struct {
	Number    int              `yaml:"number,omitempty"`
	Title     string           `yaml:"title,omitempty"`
	Author    string           `yaml:"author,omitempty"`
	Body      string           `yaml:"body,omitempty"`
	Base      string           `yaml:"base,omitempty"`
	Head      *Head            `yaml:"head,omitempty"`
	State     PullRequestState `yaml:"state,omitempty"`
	Merged    bool             `yaml:"merged,omitempty"`
	Mergeable bool             `yaml:"mergeable,omitempty"`
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

func Load(definitionFiles ...string) ([]*Team, []*User, []*Repository, error) {
	var teams []*Team
	var users []*User
	var repos []*Repository
	for _, v := range definitionFiles {
		f, err := os.Open(v)
		if err != nil {
			return nil, nil, nil, err
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
				return nil, nil, nil, err
			}

			switch k.Kind {
			case "User":
				user := &User{}
				if err := node.Decode(user); err != nil {
					return nil, nil, nil, err
				}
				users = append(users, user)
			case "Team":
				team := &Team{}
				if err := node.Decode(team); err != nil {
					return nil, nil, nil, err
				}
				teams = append(teams, team)
			default:
				repo := &Repository{}
				if err := node.Decode(repo); err != nil {
					return nil, nil, nil, err
				}
				repos = append(repos, repo)
			}
		}
	}
	return teams, users, repos, nil
}
