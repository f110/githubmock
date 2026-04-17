package githubmock

const graphqlSchema = `
schema {
	query: Query
}

type Query {
	repository(owner: String!, name: String!): Repository
	user(login: String!): User
}

type Repository {
	id: ID!
	name: String!
	owner: RepositoryOwner!
	defaultBranchRef: Ref
	pullRequests(
		first: Int
		after: String
		states: [PullRequestState!]
	): PullRequestConnection!
	pullRequest(number: Int!): PullRequest
	issues(
		first: Int
		after: String
		states: [IssueState!]
	): IssueConnection!
	issue(number: Int!): Issue
}

type RepositoryOwner {
	login: String!
	avatarUrl: String!
}

type User {
	id: ID!
	login: String!
	name: String
	avatarUrl: String!
}

type Ref {
	name: String!
}

type PullRequest {
	id: ID!
	number: Int!
	title: String!
	body: String!
	state: PullRequestState!
	merged: Boolean!
	mergeable: MergeableState!
	author: Actor
	baseRefName: String!
	headRefName: String!
	createdAt: String!
	updatedAt: String!
	comments(first: Int, after: String): IssueCommentConnection!
	reviews(first: Int, after: String): PullRequestReviewConnection!
}

type Issue {
	id: ID!
	number: Int!
	title: String!
	body: String!
	state: IssueState!
	author: Actor
	createdAt: String!
	updatedAt: String!
	comments(first: Int, after: String): IssueCommentConnection!
}

type Actor {
	login: String!
	avatarUrl: String!
}

enum PullRequestState {
	OPEN
	CLOSED
	MERGED
}

enum IssueState {
	OPEN
	CLOSED
}

enum MergeableState {
	MERGEABLE
	CONFLICTING
	UNKNOWN
}

enum PullRequestReviewState {
	PENDING
	COMMENTED
	APPROVED
	CHANGES_REQUESTED
	DISMISSED
}

type PageInfo {
	hasNextPage: Boolean!
	hasPreviousPage: Boolean!
	startCursor: String
	endCursor: String
}

type PullRequestConnection {
	nodes: [PullRequest]!
	edges: [PullRequestEdge]!
	totalCount: Int!
	pageInfo: PageInfo!
}

type PullRequestEdge {
	node: PullRequest
	cursor: String!
}

type IssueConnection {
	nodes: [Issue]!
	edges: [IssueEdge]!
	totalCount: Int!
	pageInfo: PageInfo!
}

type IssueEdge {
	node: Issue
	cursor: String!
}

type IssueCommentConnection {
	nodes: [IssueComment]!
	totalCount: Int!
	pageInfo: PageInfo!
}

type IssueComment {
	id: ID!
	body: String!
	author: Actor
}

type PullRequestReviewConnection {
	nodes: [PullRequestReview]!
	totalCount: Int!
	pageInfo: PageInfo!
}

type PullRequestReview {
	id: ID!
	body: String!
	state: PullRequestReviewState!
	author: Actor
	submittedAt: String
}
`
