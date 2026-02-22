package githubmock

import (
	"github.com/google/go-github/v83/github"
)

type Commit struct {
	ghCommit   *github.Commit
	isHead     bool
	files      []*File
	parents    []*Commit
	ghStatuses []*github.RepoStatus
}

func NewCommit() *Commit {
	return &Commit{
		ghCommit: &github.Commit{},
		files:    []*File{{Name: "", sha: newHash(), mode: fileTypeDir}}, // Root directory
	}
}

func (c *Commit) SHA(v string) *Commit {
	if v == "" {
		return c
	}
	c.ghCommit.SHA = new(v)
	return c
}

func (c *Commit) Parents(parents ...*Commit) *Commit {
	for _, p := range parents {
		c.ghCommit.Parents = append(c.ghCommit.Parents, p.ghCommit)
	}
	return c
}

func (c *Commit) Files(files ...*File) *Commit {
	c.files = append(c.files, files...)
	return c
}

func (c *Commit) IsHead() *Commit {
	c.isHead = true
	return c
}

func (c *Commit) Statuses(statuses ...*CommitStatus) *Commit {
	for _, v := range statuses {
		c.ghStatuses = append(c.ghStatuses, &github.RepoStatus{
			State:       new(string(v.State)),
			Description: new(v.Description),
		})
	}
	return c
}

func (c *Commit) addDir(dir string) {
	for _, v := range c.files {
		if v.Name == dir {
			return
		}
	}
	c.files = append(c.files, &File{
		Name: dir,
		sha:  newHash(),
		mode: fileTypeDir,
	})
}

func (c *Commit) addFile(file *File) {
	if file.sha == "" {
		file.sha = newHash()
	}
	file.mode = fileTypeRegular
}

type fileType int

const (
	fileTypeRegular fileType = iota
	fileTypeDir
)

type File struct {
	Name string
	Body []byte

	sha  string
	mode fileType
}

type CommitState string

const (
	CommitStatePending CommitState = "pending"
	CommitStateSuccess CommitState = "success"
	CommitStateFailure CommitState = "failure"
	CommitStateError   CommitState = "error"
)

type CommitStatus struct {
	State       CommitState
	Description string
}

type PullRequest struct {
	ghPullRequest *github.PullRequest
	headRepo      *Repository
	comments      []*github.PullRequestComment
	reviews       []*github.PullRequestReview
}

func NewPullRequest() *PullRequest {
	return &PullRequest{ghPullRequest: &github.PullRequest{}}
}

func (pr *PullRequest) Number(v int) *PullRequest {
	if v <= 0 {
		return pr
	}
	pr.ghPullRequest.Number = new(v)
	return pr
}

func (pr *PullRequest) Title(v string) *PullRequest {
	if v == "" {
		return pr
	}
	pr.ghPullRequest.Title = new(v)
	return pr
}

func (pr *PullRequest) Body(v string) *PullRequest {
	if v == "" {
		return pr
	}
	pr.ghPullRequest.Body = new(v)
	return pr
}

func (pr *PullRequest) Base(ref string) *PullRequest {
	if ref == "" {
		return pr
	}
	pr.ghPullRequest.Base = &github.PullRequestBranch{Ref: new(ref)}
	return pr
}

func (pr *PullRequest) Head(repo *Repository, ref string) *PullRequest {
	if repo == nil || ref == "" {
		return pr
	}
	pr.headRepo = repo
	pr.ghPullRequest.Head = &github.PullRequestBranch{Ref: new(ref)}
	return pr
}

func (pr *PullRequest) Comments(comments []*Comment) *PullRequest {
	for _, v := range comments {
		pr.comments = append(pr.comments, &github.PullRequestComment{
			User: &github.User{Login: new(v.Author)},
			Body: new(v.Body),
		})
	}
	return pr
}

type Issue struct {
	ghIssue  *github.Issue
	comments []*github.IssueComment
}

func NewIssue() *Issue {
	return &Issue{ghIssue: &github.Issue{}}
}

func (i *Issue) Number(v int) *Issue {
	if v <= 0 {
		return i
	}
	i.ghIssue.Number = new(v)
	return i
}

func (i *Issue) Title(v string) *Issue {
	if v == "" {
		return i
	}
	i.ghIssue.Title = new(v)
	return i
}

func (i *Issue) Comments(comments []*Comment) *Issue {
	for _, v := range comments {
		i.comments = append(i.comments, &github.IssueComment{
			User: &github.User{Login: new(v.Author)},
			Body: new(v.Body),
		})
	}
	return i
}

type Tag struct {
	ghTag  *github.Tag
	commit *Commit
}

func NewTag() *Tag {
	return &Tag{ghTag: &github.Tag{}}
}

func (t *Tag) Name(v string) *Tag {
	if v == "" {
		return t
	}
	t.ghTag.Tag = new(v)
	return t
}

func (t *Tag) Commit(c *Commit) *Tag {
	if c == nil {
		return t
	}
	t.commit = c
	t.ghTag.SHA = c.ghCommit.SHA
	return t
}

func (t *Tag) toGithubTag() *github.Tag {
	t.ghTag.SHA = t.commit.ghCommit.SHA
	return t.ghTag
}

type Comment struct {
	Author string
	Body   string
}
