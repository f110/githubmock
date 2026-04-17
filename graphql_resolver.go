package githubmock

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/graph-gophers/graphql-go"
)

// rootResolver is the top-level GraphQL resolver.
type rootResolver struct {
	mock *Mock
}

func (r *rootResolver) Repository(args struct{ Owner, Name string }) *repositoryResolver {
	r.mock.mu.Lock()
	defer r.mock.mu.Unlock()

	key := fmt.Sprintf("%s/%s", args.Owner, args.Name)
	repo, ok := r.mock.repositories[key]
	if !ok {
		return nil
	}
	return &repositoryResolver{mock: r.mock, repo: repo, owner: args.Owner, name: args.Name}
}

func (r *rootResolver) User(args struct{ Login string }) *userResolver {
	r.mock.mu.Lock()
	defer r.mock.mu.Unlock()

	u, ok := r.mock.users[args.Login]
	if !ok {
		return nil
	}
	return &userResolver{user: u}
}

// repositoryResolver resolves Repository fields.
type repositoryResolver struct {
	mock  *Mock
	repo  *Repository
	owner string
	name  string
}

func (r *repositoryResolver) ID() graphql.ID {
	return graphql.ID(encodeID("Repository", fmt.Sprintf("%s/%s", r.owner, r.name)))
}

func (r *repositoryResolver) Name() string {
	return r.repo.ghRepository.GetName()
}

func (r *repositoryResolver) Owner() *repositoryOwnerResolver {
	if r.repo.ghRepository.Owner == nil {
		return &repositoryOwnerResolver{}
	}
	return &repositoryOwnerResolver{
		login:     r.repo.ghRepository.Owner.GetLogin(),
		avatarURL: r.repo.ghRepository.Owner.GetAvatarURL(),
	}
}

func (r *repositoryResolver) DefaultBranchRef() *refResolver {
	branch := r.repo.ghRepository.GetDefaultBranch()
	if branch == "" {
		return nil
	}
	return &refResolver{name: branch}
}

func (r *repositoryResolver) PullRequests(args struct {
	First  *int32
	After  *string
	States *[]string
}) *pullRequestConnectionResolver {
	r.repo.mu.Lock()
	defer r.repo.mu.Unlock()

	var filtered []*PullRequest
	for _, pr := range r.repo.pullRequests {
		if args.States != nil && len(*args.States) > 0 {
			state := prStateToGraphQL(pr)
			matched := false
			for _, s := range *args.States {
				if s == state {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, pr)
	}
	return &pullRequestConnectionResolver{
		prs:   filtered,
		owner: r.owner,
		name:  r.name,
		first: args.First,
		after: args.After,
	}
}

func (r *repositoryResolver) PullRequest(args struct{ Number int32 }) *pullRequestResolver {
	pr := r.repo.GetPullRequest(int(args.Number))
	if pr == nil {
		return nil
	}
	return &pullRequestResolver{pr: pr, owner: r.owner, name: r.name}
}

func (r *repositoryResolver) Issues(args struct {
	First  *int32
	After  *string
	States *[]string
}) *issueConnectionResolver {
	r.repo.mu.Lock()
	defer r.repo.mu.Unlock()

	var filtered []*Issue
	for _, issue := range r.repo.issues {
		if args.States != nil && len(*args.States) > 0 {
			state := issueStateToGraphQL(issue)
			matched := false
			for _, s := range *args.States {
				if s == state {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, issue)
	}
	return &issueConnectionResolver{
		issues: filtered,
		owner:  r.owner,
		name:   r.name,
		first:  args.First,
		after:  args.After,
	}
}

func (r *repositoryResolver) Issue(args struct{ Number int32 }) *issueResolver {
	issue := r.repo.GetIssue(int(args.Number))
	if issue == nil {
		return nil
	}
	return &issueResolver{issue: issue, owner: r.owner, name: r.name}
}

// repositoryOwnerResolver resolves RepositoryOwner fields.
type repositoryOwnerResolver struct {
	login     string
	avatarURL string
}

func (r *repositoryOwnerResolver) Login() string    { return r.login }
func (r *repositoryOwnerResolver) AvatarUrl() string { return r.avatarURL }

// refResolver resolves Ref fields.
type refResolver struct {
	name string
}

func (r *refResolver) Name() string { return r.name }

// userResolver resolves User fields.
type userResolver struct {
	user *User
}

func (r *userResolver) ID() graphql.ID {
	return graphql.ID(encodeID("User", r.user.ghUser.GetLogin()))
}

func (r *userResolver) Login() string    { return r.user.ghUser.GetLogin() }
func (r *userResolver) Name() *string    { return r.user.ghUser.Name }
func (r *userResolver) AvatarUrl() string { return r.user.ghUser.GetAvatarURL() }

// actorResolver resolves Actor fields.
type actorResolver struct {
	login     string
	avatarURL string
}

func (r *actorResolver) Login() string    { return r.login }
func (r *actorResolver) AvatarUrl() string { return r.avatarURL }

// pullRequestResolver resolves PullRequest fields.
type pullRequestResolver struct {
	pr    *PullRequest
	owner string
	name  string
}

func (r *pullRequestResolver) ID() graphql.ID {
	return graphql.ID(encodeID("PullRequest", fmt.Sprintf("%s/%s:%d", r.owner, r.name, r.pr.ghPullRequest.GetNumber())))
}

func (r *pullRequestResolver) Number() int32 { return int32(r.pr.ghPullRequest.GetNumber()) }
func (r *pullRequestResolver) Title() string  { return r.pr.ghPullRequest.GetTitle() }
func (r *pullRequestResolver) Body() string   { return r.pr.ghPullRequest.GetBody() }

func (r *pullRequestResolver) State() string {
	return prStateToGraphQL(r.pr)
}

func (r *pullRequestResolver) Merged() bool { return r.pr.ghPullRequest.GetMerged() }

func (r *pullRequestResolver) Mergeable() string {
	if r.pr.ghPullRequest.GetMergeable() {
		return "MERGEABLE"
	}
	return "UNKNOWN"
}

func (r *pullRequestResolver) Author() *actorResolver {
	u := r.pr.ghPullRequest.GetUser()
	if u == nil {
		return nil
	}
	return &actorResolver{login: u.GetLogin(), avatarURL: u.GetAvatarURL()}
}

func (r *pullRequestResolver) BaseRefName() string {
	if r.pr.ghPullRequest.Base != nil {
		return r.pr.ghPullRequest.Base.GetRef()
	}
	return ""
}

func (r *pullRequestResolver) HeadRefName() string {
	if r.pr.ghPullRequest.Head != nil {
		return r.pr.ghPullRequest.Head.GetRef()
	}
	return ""
}

func (r *pullRequestResolver) CreatedAt() string {
	return r.pr.ghPullRequest.GetCreatedAt().Format(time.RFC3339)
}

func (r *pullRequestResolver) UpdatedAt() string {
	return r.pr.ghPullRequest.GetUpdatedAt().Format(time.RFC3339)
}

func (r *pullRequestResolver) Comments(args struct {
	First *int32
	After *string
}) *issueCommentConnectionResolver {
	var comments []commentData
	for i, c := range r.pr.comments {
		comments = append(comments, commentData{
			id:   encodeID("IssueComment", fmt.Sprintf("%s/%s:pr%d:%d", r.owner, r.name, r.pr.ghPullRequest.GetNumber(), i)),
			body: c.GetBody(),
			author: func() *actorResolver {
				u := c.GetUser()
				if u == nil {
					return nil
				}
				return &actorResolver{login: u.GetLogin(), avatarURL: u.GetAvatarURL()}
			}(),
		})
	}
	return &issueCommentConnectionResolver{comments: comments, first: args.First, after: args.After}
}

func (r *pullRequestResolver) Reviews(args struct {
	First *int32
	After *string
}) *pullRequestReviewConnectionResolver {
	var reviews []reviewData
	for i, rev := range r.pr.reviews {
		reviews = append(reviews, reviewData{
			id:    encodeID("PullRequestReview", fmt.Sprintf("%s/%s:%d:%d", r.owner, r.name, r.pr.ghPullRequest.GetNumber(), i)),
			body:  rev.GetBody(),
			state: strings.ReplaceAll(strings.ToUpper(rev.GetState()), " ", "_"),
			author: func() *actorResolver {
				u := rev.GetUser()
				if u == nil {
					return nil
				}
				return &actorResolver{login: u.GetLogin(), avatarURL: u.GetAvatarURL()}
			}(),
			submittedAt: func() *string {
				if rev.SubmittedAt == nil {
					return nil
				}
				s := rev.SubmittedAt.Format(time.RFC3339)
				return &s
			}(),
		})
	}
	return &pullRequestReviewConnectionResolver{reviews: reviews, first: args.First, after: args.After}
}

// issueResolver resolves Issue fields.
type issueResolver struct {
	issue *Issue
	owner string
	name  string
}

func (r *issueResolver) ID() graphql.ID {
	return graphql.ID(encodeID("Issue", fmt.Sprintf("%s/%s:%d", r.owner, r.name, r.issue.ghIssue.GetNumber())))
}

func (r *issueResolver) Number() int32 { return int32(r.issue.ghIssue.GetNumber()) }
func (r *issueResolver) Title() string  { return r.issue.ghIssue.GetTitle() }
func (r *issueResolver) Body() string   { return r.issue.ghIssue.GetBody() }

func (r *issueResolver) State() string {
	return issueStateToGraphQL(r.issue)
}

func (r *issueResolver) Author() *actorResolver {
	u := r.issue.ghIssue.GetUser()
	if u == nil {
		return nil
	}
	return &actorResolver{login: u.GetLogin(), avatarURL: u.GetAvatarURL()}
}

func (r *issueResolver) CreatedAt() string {
	return r.issue.ghIssue.GetCreatedAt().Format(time.RFC3339)
}

func (r *issueResolver) UpdatedAt() string {
	return r.issue.ghIssue.GetUpdatedAt().Format(time.RFC3339)
}

func (r *issueResolver) Comments(args struct {
	First *int32
	After *string
}) *issueCommentConnectionResolver {
	var comments []commentData
	for i, c := range r.issue.comments {
		comments = append(comments, commentData{
			id:   encodeID("IssueComment", fmt.Sprintf("%s/%s:issue%d:%d", r.owner, r.name, r.issue.ghIssue.GetNumber(), i)),
			body: c.GetBody(),
			author: func() *actorResolver {
				u := c.GetUser()
				if u == nil {
					return nil
				}
				return &actorResolver{login: u.GetLogin(), avatarURL: u.GetAvatarURL()}
			}(),
		})
	}
	return &issueCommentConnectionResolver{comments: comments, first: args.First, after: args.After}
}

// Connection resolvers

type pullRequestConnectionResolver struct {
	prs   []*PullRequest
	owner string
	name  string
	first *int32
	after *string
}

func (r *pullRequestConnectionResolver) TotalCount() int32 { return int32(len(r.prs)) }

func (r *pullRequestConnectionResolver) Nodes() []*pullRequestResolver {
	items := paginate(r.prs, r.first, r.after)
	resolvers := make([]*pullRequestResolver, len(items))
	for i, pr := range items {
		resolvers[i] = &pullRequestResolver{pr: pr, owner: r.owner, name: r.name}
	}
	return resolvers
}

func (r *pullRequestConnectionResolver) Edges() []*pullRequestEdgeResolver {
	start, items := paginateWithOffset(r.prs, r.first, r.after)
	edges := make([]*pullRequestEdgeResolver, len(items))
	for i, pr := range items {
		edges[i] = &pullRequestEdgeResolver{
			pr:     &pullRequestResolver{pr: pr, owner: r.owner, name: r.name},
			cursor: encodeCursor(start + i),
		}
	}
	return edges
}

func (r *pullRequestConnectionResolver) PageInfo() *pageInfoResolver {
	return newPageInfo(len(r.prs), r.first, r.after)
}

type pullRequestEdgeResolver struct {
	pr     *pullRequestResolver
	cursor string
}

func (r *pullRequestEdgeResolver) Node() *pullRequestResolver { return r.pr }
func (r *pullRequestEdgeResolver) Cursor() string              { return r.cursor }

type issueConnectionResolver struct {
	issues []*Issue
	owner  string
	name   string
	first  *int32
	after  *string
}

func (r *issueConnectionResolver) TotalCount() int32 { return int32(len(r.issues)) }

func (r *issueConnectionResolver) Nodes() []*issueResolver {
	items := paginate(r.issues, r.first, r.after)
	resolvers := make([]*issueResolver, len(items))
	for i, issue := range items {
		resolvers[i] = &issueResolver{issue: issue, owner: r.owner, name: r.name}
	}
	return resolvers
}

func (r *issueConnectionResolver) Edges() []*issueEdgeResolver {
	start, items := paginateWithOffset(r.issues, r.first, r.after)
	edges := make([]*issueEdgeResolver, len(items))
	for i, issue := range items {
		edges[i] = &issueEdgeResolver{
			issue:  &issueResolver{issue: issue, owner: r.owner, name: r.name},
			cursor: encodeCursor(start + i),
		}
	}
	return edges
}

func (r *issueConnectionResolver) PageInfo() *pageInfoResolver {
	return newPageInfo(len(r.issues), r.first, r.after)
}

type issueEdgeResolver struct {
	issue  *issueResolver
	cursor string
}

func (r *issueEdgeResolver) Node() *issueResolver { return r.issue }
func (r *issueEdgeResolver) Cursor() string        { return r.cursor }

// Comment connection

type commentData struct {
	id     string
	body   string
	author *actorResolver
}

type issueCommentConnectionResolver struct {
	comments []commentData
	first    *int32
	after    *string
}

func (r *issueCommentConnectionResolver) TotalCount() int32 { return int32(len(r.comments)) }

func (r *issueCommentConnectionResolver) Nodes() []*issueCommentResolver {
	items := paginate(r.comments, r.first, r.after)
	resolvers := make([]*issueCommentResolver, len(items))
	for i, c := range items {
		resolvers[i] = &issueCommentResolver{data: c}
	}
	return resolvers
}

func (r *issueCommentConnectionResolver) PageInfo() *pageInfoResolver {
	return newPageInfo(len(r.comments), r.first, r.after)
}

type issueCommentResolver struct {
	data commentData
}

func (r *issueCommentResolver) ID() graphql.ID        { return graphql.ID(r.data.id) }
func (r *issueCommentResolver) Body() string          { return r.data.body }
func (r *issueCommentResolver) Author() *actorResolver { return r.data.author }

// Review connection

type reviewData struct {
	id          string
	body        string
	state       string
	author      *actorResolver
	submittedAt *string
}

type pullRequestReviewConnectionResolver struct {
	reviews []reviewData
	first   *int32
	after   *string
}

func (r *pullRequestReviewConnectionResolver) TotalCount() int32 { return int32(len(r.reviews)) }

func (r *pullRequestReviewConnectionResolver) Nodes() []*pullRequestReviewResolver {
	items := paginate(r.reviews, r.first, r.after)
	resolvers := make([]*pullRequestReviewResolver, len(items))
	for i, rev := range items {
		resolvers[i] = &pullRequestReviewResolver{data: rev}
	}
	return resolvers
}

func (r *pullRequestReviewConnectionResolver) PageInfo() *pageInfoResolver {
	return newPageInfo(len(r.reviews), r.first, r.after)
}

type pullRequestReviewResolver struct {
	data reviewData
}

func (r *pullRequestReviewResolver) ID() graphql.ID        { return graphql.ID(r.data.id) }
func (r *pullRequestReviewResolver) Body() string          { return r.data.body }
func (r *pullRequestReviewResolver) State() string         { return r.data.state }
func (r *pullRequestReviewResolver) Author() *actorResolver { return r.data.author }
func (r *pullRequestReviewResolver) SubmittedAt() *string   { return r.data.submittedAt }

// PageInfo resolver

type pageInfoResolver struct {
	hasNextPage     bool
	hasPreviousPage bool
	startCursor     *string
	endCursor       *string
}

func (r *pageInfoResolver) HasNextPage() bool      { return r.hasNextPage }
func (r *pageInfoResolver) HasPreviousPage() bool   { return r.hasPreviousPage }
func (r *pageInfoResolver) StartCursor() *string    { return r.startCursor }
func (r *pageInfoResolver) EndCursor() *string      { return r.endCursor }

// Pagination helpers

func encodeCursor(index int) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("cursor:%d", index)))
}

func decodeCursor(cursor string) int {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0
	}
	s := string(b)
	if !strings.HasPrefix(s, "cursor:") {
		return 0
	}
	n, err := strconv.Atoi(s[len("cursor:"):])
	if err != nil {
		return 0
	}
	return n
}

func paginate[T any](items []T, first *int32, after *string) []T {
	_, result := paginateWithOffset(items, first, after)
	return result
}

func paginateWithOffset[T any](items []T, first *int32, after *string) (int, []T) {
	start := 0
	if after != nil {
		start = decodeCursor(*after) + 1
	}
	if start > len(items) {
		start = len(items)
	}
	end := len(items)
	if first != nil {
		end = start + int(*first)
		if end > len(items) {
			end = len(items)
		}
	}
	return start, items[start:end]
}

func newPageInfo(total int, first *int32, after *string) *pageInfoResolver {
	start := 0
	if after != nil {
		start = decodeCursor(*after) + 1
	}
	if start > total {
		start = total
	}
	end := total
	if first != nil {
		end = start + int(*first)
		if end > total {
			end = total
		}
	}

	pi := &pageInfoResolver{
		hasNextPage:     end < total,
		hasPreviousPage: start > 0,
	}
	if start < end {
		sc := encodeCursor(start)
		ec := encodeCursor(end - 1)
		pi.startCursor = &sc
		pi.endCursor = &ec
	}
	return pi
}

func encodeID(typeName, id string) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", typeName, id)))
}

// State conversion helpers

func prStateToGraphQL(pr *PullRequest) string {
	if pr.ghPullRequest.GetMerged() {
		return "MERGED"
	}
	switch pr.ghPullRequest.GetState() {
	case "closed":
		return "CLOSED"
	default:
		return "OPEN"
	}
}

func issueStateToGraphQL(issue *Issue) string {
	switch issue.ghIssue.GetState() {
	case "closed":
		return "CLOSED"
	default:
		return "OPEN"
	}
}
