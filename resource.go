package githubmock

import (
	"time"

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

type PullRequestState string

const (
	PullRequestStateOpen   PullRequestState = "open"
	PullRequestStateClosed PullRequestState = "closed"
)

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

func (pr *PullRequest) Author(v string) *PullRequest {
	if v == "" {
		return pr
	}
	pr.ghPullRequest.User = &github.User{Login: new(v)}
	return pr
}

func (pr *PullRequest) State(v PullRequestState) *PullRequest {
	if v == "" {
		return pr
	}
	pr.ghPullRequest.State = new(string(v))
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

func (pr *PullRequest) Comments(comments ...*Comment) *PullRequest {
	for _, v := range comments {
		pr.comments = append(pr.comments, &github.PullRequestComment{
			User: &github.User{Login: new(v.Author)},
			Body: new(v.Body),
		})
	}
	return pr
}

func (pr *PullRequest) Reviews(reviews ...*Review) *PullRequest {
	for _, v := range reviews {
		pr.reviews = append(pr.reviews, v.ghReview)
	}
	return pr
}

func (pr *PullRequest) CreatedAt(t time.Time) *PullRequest {
	if t.IsZero() {
		return pr
	}
	pr.ghPullRequest.CreatedAt = &github.Timestamp{Time: t}
	return pr
}

func (pr *PullRequest) UpdatedAt(t time.Time) *PullRequest {
	if t.IsZero() {
		return pr
	}
	pr.ghPullRequest.UpdatedAt = &github.Timestamp{Time: t}
	return pr
}

type IssueState string

const (
	IssueStateOpen   IssueState = "open"
	IssueStateClosed IssueState = "closed"
)

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

func (i *Issue) Author(v string) *Issue {
	if v == "" {
		return i
	}
	i.ghIssue.User = &github.User{Login: new(v)}
	return i
}

func (i *Issue) State(v IssueState) *Issue {
	if v == "" {
		return i
	}
	i.ghIssue.State = new(string(v))
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

func (i *Issue) CreatedAt(t time.Time) *Issue {
	if t.IsZero() {
		return i
	}
	i.ghIssue.CreatedAt = &github.Timestamp{Time: t}
	return i
}

func (i *Issue) UpdatedAt(t time.Time) *Issue {
	if t.IsZero() {
		return i
	}
	i.ghIssue.UpdatedAt = &github.Timestamp{Time: t}
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

type ReviewState string

const (
	ReviewStateChangesRequested ReviewState = "CHANGES_REQUESTED"
	ReviewStateCommented        ReviewState = "COMMENTED"
	ReviewStateApproved         ReviewState = "APPROVED"
)

type Review struct {
	ghReview *github.PullRequestReview
}

func NewReview() *Review {
	return &Review{ghReview: &github.PullRequestReview{}}
}

func (r *Review) Body(v string) *Review {
	if v == "" {
		return r
	}
	r.ghReview.Body = new(v)
	return r
}

func (r *Review) Author(v string) *Review {
	if v == "" {
		return r
	}
	r.ghReview.User = &github.User{Login: new(v)}
	return r
}

func (r *Review) State(v ReviewState) *Review {
	if v == "" {
		return r
	}
	r.ghReview.State = new(string(v))
	return r
}

func (r *Review) SubmittedAt(t time.Time) *Review {
	if t.IsZero() {
		return r
	}
	r.ghReview.SubmittedAt = &github.Timestamp{Time: t}
	return r
}
