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

type PullRequest struct {
	ghPullRequest *github.PullRequest
	headRepo      *Repository

	Comments []*github.PullRequestComment `json:"-"`
}

func NewPullRequest() *PullRequest {
	return &PullRequest{ghPullRequest: &github.PullRequest{}}
}

func (pr *PullRequest) Number(v int) *PullRequest {
	pr.ghPullRequest.Number = new(v)
	return pr
}

func (pr *PullRequest) Title(v string) *PullRequest {
	pr.ghPullRequest.Title = new(v)
	return pr
}

func (pr *PullRequest) Body(v string) *PullRequest {
	pr.ghPullRequest.Body = new(v)
	return pr
}

func (pr *PullRequest) Base(ref string) *PullRequest {
	pr.ghPullRequest.Base = &github.PullRequestBranch{Ref: new(ref)}
	return pr
}

func (pr *PullRequest) Head(repo *Repository, ref string) *PullRequest {
	pr.headRepo = repo
	pr.ghPullRequest.Head = &github.PullRequestBranch{Ref: new(ref)}
	return pr
}

type Issue struct {
	ghIssue  *github.Issue
	Comments []*github.IssueComment `json:"-"`
}

func NewIssue() *Issue {
	return &Issue{ghIssue: &github.Issue{}}
}

func (i *Issue) Number(v int) *Issue {
	i.ghIssue.Number = new(v)
	return i
}

func (i *Issue) Title(v string) *Issue {
	i.ghIssue.Title = new(v)
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
	t.ghTag.Tag = new(v)
	return t
}

func (t *Tag) Commit(c *Commit) *Tag {
	t.commit = c
	t.ghTag.SHA = c.ghCommit.SHA
	return t
}

func (t *Tag) toGithubTag() *github.Tag {
	t.ghTag.SHA = t.commit.ghCommit.SHA
	return t.ghTag
}
