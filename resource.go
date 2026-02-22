package githubmock

import (
	"github.com/google/go-github/v83/github"
)

type Commit struct {
	Hash    string    `json:"-"`
	Parents []*Commit `json:"-"`
	Files   []*File   `json:"-"`
	IsHead  bool      `json:"-"`

	files      []*File
	ghCommit   *github.Commit
	ghStatuses []*github.RepoStatus
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
	c.files = append(c.files, file)
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
	github.PullRequest

	Comments []*github.PullRequestComment `json:"-"`
}

type Issue struct {
	github.Issue

	Comments []*github.IssueComment `json:"-"`
}

type Tag struct {
	Name   string
	Commit *Commit

	ghTag *github.Tag
}
