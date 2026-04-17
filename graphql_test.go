package githubmock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

func execGraphQL(t *testing.T, m *Mock, query string, variables map[string]any) graphqlResponse {
	t.Helper()
	mux := http.NewServeMux()
	m.RegisterHandler(mux)

	body, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp graphqlResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	return resp
}

func TestGraphQL(t *testing.T) {
	t.Run("Repository", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("f110/gh-test")
		repo.DefaultBranch("main")

		resp := execGraphQL(t, m, `{
			repository(owner: "f110", name: "gh-test") {
				name
				owner { login }
				defaultBranchRef { name }
			}
		}`, nil)
		require.Empty(t, resp.Errors)

		var data struct {
			Repository struct {
				Name  string
				Owner struct{ Login string }
				DefaultBranchRef struct{ Name string }
			}
		}
		require.NoError(t, json.Unmarshal(resp.Data, &data))
		assert.Equal(t, "gh-test", data.Repository.Name)
		assert.Equal(t, "f110", data.Repository.Owner.Login)
		assert.Equal(t, "main", data.Repository.DefaultBranchRef.Name)
	})

	t.Run("RepositoryNotFound", func(t *testing.T) {
		m := NewMock()
		resp := execGraphQL(t, m, `{
			repository(owner: "no", name: "exist") {
				name
			}
		}`, nil)
		require.Empty(t, resp.Errors)

		var data struct {
			Repository *struct{ Name string }
		}
		require.NoError(t, json.Unmarshal(resp.Data, &data))
		assert.Nil(t, data.Repository)
	})

	t.Run("PullRequests", func(t *testing.T) {
		m := NewMock()
		user := m.User("octocat")
		repo := m.Repository("f110/gh-test")
		repo.PullRequests(
			NewPullRequest().
				Number(1).
				Title("Fix bug").
				Body("Bug fix description").
				Author(user).
				State(PullRequestStateOpen).
				Base("main").
				Head(nil, "fix-branch").
				Mergeable().
				CreatedAt(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).
				UpdatedAt(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
			NewPullRequest().
				Number(2).
				Title("Add feature").
				State(PullRequestStateClosed).
				Merged().
				Base("main").
				Head(nil, "feature-branch").
				CreatedAt(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)).
				UpdatedAt(time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)),
			NewPullRequest().
				Number(3).
				Title("WIP").
				State(PullRequestStateOpen).
				Base("main").
				Head(nil, "wip").
				CreatedAt(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)).
				UpdatedAt(time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)),
		)

		t.Run("ListAll", func(t *testing.T) {
			resp := execGraphQL(t, m, `{
				repository(owner: "f110", name: "gh-test") {
					pullRequests(first: 10) {
						totalCount
						nodes {
							number
							title
							state
						}
					}
				}
			}`, nil)
			require.Empty(t, resp.Errors)

			var data struct {
				Repository struct {
					PullRequests struct {
						TotalCount int
						Nodes      []struct {
							Number int
							Title  string
							State  string
						}
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp.Data, &data))
			assert.Equal(t, 3, data.Repository.PullRequests.TotalCount)
			assert.Len(t, data.Repository.PullRequests.Nodes, 3)
		})

		t.Run("FilterByState", func(t *testing.T) {
			resp := execGraphQL(t, m, `{
				repository(owner: "f110", name: "gh-test") {
					pullRequests(first: 10, states: [OPEN]) {
						totalCount
						nodes {
							number
							state
						}
					}
				}
			}`, nil)
			require.Empty(t, resp.Errors)

			var data struct {
				Repository struct {
					PullRequests struct {
						TotalCount int
						Nodes      []struct {
							Number int
							State  string
						}
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp.Data, &data))
			assert.Equal(t, 2, data.Repository.PullRequests.TotalCount)
			for _, node := range data.Repository.PullRequests.Nodes {
				assert.Equal(t, "OPEN", node.State)
			}
		})

		t.Run("FilterMerged", func(t *testing.T) {
			resp := execGraphQL(t, m, `{
				repository(owner: "f110", name: "gh-test") {
					pullRequests(first: 10, states: [MERGED]) {
						totalCount
						nodes {
							number
							title
							state
							merged
						}
					}
				}
			}`, nil)
			require.Empty(t, resp.Errors)

			var data struct {
				Repository struct {
					PullRequests struct {
						TotalCount int
						Nodes      []struct {
							Number int
							Title  string
							State  string
							Merged bool
						}
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp.Data, &data))
			assert.Equal(t, 1, data.Repository.PullRequests.TotalCount)
			assert.Equal(t, "MERGED", data.Repository.PullRequests.Nodes[0].State)
			assert.True(t, data.Repository.PullRequests.Nodes[0].Merged)
		})

		t.Run("SinglePullRequest", func(t *testing.T) {
			resp := execGraphQL(t, m, `{
				repository(owner: "f110", name: "gh-test") {
					pullRequest(number: 1) {
						number
						title
						body
						state
						merged
						mergeable
						author { login }
						baseRefName
						headRefName
						createdAt
						updatedAt
					}
				}
			}`, nil)
			require.Empty(t, resp.Errors)

			var data struct {
				Repository struct {
					PullRequest struct {
						Number      int
						Title       string
						Body        string
						State       string
						Merged      bool
						Mergeable   string
						Author      struct{ Login string }
						BaseRefName string
						HeadRefName string
						CreatedAt   string
						UpdatedAt   string
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp.Data, &data))
			pr := data.Repository.PullRequest
			assert.Equal(t, 1, pr.Number)
			assert.Equal(t, "Fix bug", pr.Title)
			assert.Equal(t, "Bug fix description", pr.Body)
			assert.Equal(t, "OPEN", pr.State)
			assert.False(t, pr.Merged)
			assert.Equal(t, "MERGEABLE", pr.Mergeable)
			assert.Equal(t, "octocat", pr.Author.Login)
			assert.Equal(t, "main", pr.BaseRefName)
			assert.Equal(t, "fix-branch", pr.HeadRefName)
			assert.Equal(t, "2024-01-01T00:00:00Z", pr.CreatedAt)
			assert.Equal(t, "2024-01-02T00:00:00Z", pr.UpdatedAt)
		})

		t.Run("Pagination", func(t *testing.T) {
			resp := execGraphQL(t, m, `{
				repository(owner: "f110", name: "gh-test") {
					pullRequests(first: 2) {
						totalCount
						pageInfo {
							hasNextPage
							hasPreviousPage
							endCursor
						}
						nodes { number }
					}
				}
			}`, nil)
			require.Empty(t, resp.Errors)

			var data struct {
				Repository struct {
					PullRequests struct {
						TotalCount int
						PageInfo   struct {
							HasNextPage     bool
							HasPreviousPage bool
							EndCursor       string
						}
						Nodes []struct{ Number int }
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp.Data, &data))
			assert.Equal(t, 3, data.Repository.PullRequests.TotalCount)
			assert.Len(t, data.Repository.PullRequests.Nodes, 2)
			assert.True(t, data.Repository.PullRequests.PageInfo.HasNextPage)
			assert.False(t, data.Repository.PullRequests.PageInfo.HasPreviousPage)

			// Fetch next page
			cursor := data.Repository.PullRequests.PageInfo.EndCursor
			resp2 := execGraphQL(t, m, `query($cursor: String) {
				repository(owner: "f110", name: "gh-test") {
					pullRequests(first: 2, after: $cursor) {
						pageInfo {
							hasNextPage
							hasPreviousPage
						}
						nodes { number }
					}
				}
			}`, map[string]any{"cursor": cursor})
			require.Empty(t, resp2.Errors)

			var data2 struct {
				Repository struct {
					PullRequests struct {
						PageInfo struct {
							HasNextPage     bool
							HasPreviousPage bool
						}
						Nodes []struct{ Number int }
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp2.Data, &data2))
			assert.Len(t, data2.Repository.PullRequests.Nodes, 1)
			assert.False(t, data2.Repository.PullRequests.PageInfo.HasNextPage)
			assert.True(t, data2.Repository.PullRequests.PageInfo.HasPreviousPage)
		})
	})

	t.Run("Issues", func(t *testing.T) {
		m := NewMock()
		user := m.User("octocat")
		repo := m.Repository("f110/gh-test")
		repo.Issues(
			NewIssue().
				Number(1).
				Title("Bug report").
				Author(user).
				State(IssueStateOpen).
				CreatedAt(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).
				UpdatedAt(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
			NewIssue().
				Number(2).
				Title("Closed issue").
				State(IssueStateClosed),
		)

		t.Run("ListAll", func(t *testing.T) {
			resp := execGraphQL(t, m, `{
				repository(owner: "f110", name: "gh-test") {
					issues(first: 10) {
						totalCount
						nodes {
							number
							title
							state
						}
					}
				}
			}`, nil)
			require.Empty(t, resp.Errors)

			var data struct {
				Repository struct {
					Issues struct {
						TotalCount int
						Nodes      []struct {
							Number int
							Title  string
							State  string
						}
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp.Data, &data))
			assert.Equal(t, 2, data.Repository.Issues.TotalCount)
		})

		t.Run("FilterByState", func(t *testing.T) {
			resp := execGraphQL(t, m, `{
				repository(owner: "f110", name: "gh-test") {
					issues(first: 10, states: [OPEN]) {
						totalCount
						nodes { number state }
					}
				}
			}`, nil)
			require.Empty(t, resp.Errors)

			var data struct {
				Repository struct {
					Issues struct {
						TotalCount int
						Nodes      []struct {
							Number int
							State  string
						}
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp.Data, &data))
			assert.Equal(t, 1, data.Repository.Issues.TotalCount)
			assert.Equal(t, "OPEN", data.Repository.Issues.Nodes[0].State)
		})

		t.Run("SingleIssue", func(t *testing.T) {
			resp := execGraphQL(t, m, `{
				repository(owner: "f110", name: "gh-test") {
					issue(number: 1) {
						number
						title
						state
						author { login }
						createdAt
						updatedAt
					}
				}
			}`, nil)
			require.Empty(t, resp.Errors)

			var data struct {
				Repository struct {
					Issue struct {
						Number    int
						Title     string
						State     string
						Author    struct{ Login string }
						CreatedAt string
						UpdatedAt string
					}
				}
			}
			require.NoError(t, json.Unmarshal(resp.Data, &data))
			assert.Equal(t, 1, data.Repository.Issue.Number)
			assert.Equal(t, "Bug report", data.Repository.Issue.Title)
			assert.Equal(t, "OPEN", data.Repository.Issue.State)
			assert.Equal(t, "octocat", data.Repository.Issue.Author.Login)
		})
	})

	t.Run("PullRequestWithReviews", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("f110/gh-test")
		repo.PullRequests(
			NewPullRequest().
				Number(1).
				Title("PR with reviews").
				State(PullRequestStateOpen).
				Base("main").
				Head(nil, "feature").
				Reviews(
					NewReview().
						Author("reviewer1").
						Body("LGTM").
						State(ReviewStateApproved).
						SubmittedAt(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)),
					NewReview().
						Author("reviewer2").
						Body("Needs changes").
						State(ReviewStateChangesRequested),
				),
		)

		resp := execGraphQL(t, m, `{
			repository(owner: "f110", name: "gh-test") {
				pullRequest(number: 1) {
					reviews(first: 10) {
						totalCount
						nodes {
							body
							state
							author { login }
							submittedAt
						}
					}
				}
			}
		}`, nil)
		require.Empty(t, resp.Errors)

		var data struct {
			Repository struct {
				PullRequest struct {
					Reviews struct {
						TotalCount int
						Nodes      []struct {
							Body        string
							State       string
							Author      struct{ Login string }
							SubmittedAt *string
						}
					}
				}
			}
		}
		require.NoError(t, json.Unmarshal(resp.Data, &data))
		assert.Equal(t, 2, data.Repository.PullRequest.Reviews.TotalCount)
		assert.Equal(t, "LGTM", data.Repository.PullRequest.Reviews.Nodes[0].Body)
		assert.Equal(t, "APPROVED", data.Repository.PullRequest.Reviews.Nodes[0].State)
		assert.Equal(t, "reviewer1", data.Repository.PullRequest.Reviews.Nodes[0].Author.Login)
		require.NotNil(t, data.Repository.PullRequest.Reviews.Nodes[0].SubmittedAt)
		assert.Equal(t, "2024-01-01T12:00:00Z", *data.Repository.PullRequest.Reviews.Nodes[0].SubmittedAt)
		assert.Equal(t, "CHANGES_REQUESTED", data.Repository.PullRequest.Reviews.Nodes[1].State)
	})

	t.Run("User", func(t *testing.T) {
		m := NewMock()
		m.User("octocat").Name("The Octocat").AvatarURL("https://example.com/avatar.png")

		resp := execGraphQL(t, m, `{
			user(login: "octocat") {
				login
				name
				avatarUrl
			}
		}`, nil)
		require.Empty(t, resp.Errors)

		var data struct {
			User struct {
				Login     string
				Name      string
				AvatarUrl string
			}
		}
		require.NoError(t, json.Unmarshal(resp.Data, &data))
		assert.Equal(t, "octocat", data.User.Login)
		assert.Equal(t, "The Octocat", data.User.Name)
		assert.Equal(t, "https://example.com/avatar.png", data.User.AvatarUrl)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		m := NewMock()
		resp := execGraphQL(t, m, `{
			user(login: "nobody") {
				login
			}
		}`, nil)
		require.Empty(t, resp.Errors)

		var data struct {
			User *struct{ Login string }
		}
		require.NoError(t, json.Unmarshal(resp.Data, &data))
		assert.Nil(t, data.User)
	})

	t.Run("Edges", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("f110/gh-test")
		repo.PullRequests(
			NewPullRequest().Number(1).Title("PR 1").State(PullRequestStateOpen).Base("main").Head(nil, "b1"),
			NewPullRequest().Number(2).Title("PR 2").State(PullRequestStateOpen).Base("main").Head(nil, "b2"),
		)

		resp := execGraphQL(t, m, `{
			repository(owner: "f110", name: "gh-test") {
				pullRequests(first: 10) {
					edges {
						cursor
						node { number title }
					}
				}
			}
		}`, nil)
		require.Empty(t, resp.Errors)

		var data struct {
			Repository struct {
				PullRequests struct {
					Edges []struct {
						Cursor string
						Node   struct {
							Number int
							Title  string
						}
					}
				}
			}
		}
		require.NoError(t, json.Unmarshal(resp.Data, &data))
		assert.Len(t, data.Repository.PullRequests.Edges, 2)
		assert.NotEmpty(t, data.Repository.PullRequests.Edges[0].Cursor)
		assert.Equal(t, 1, data.Repository.PullRequests.Edges[0].Node.Number)
		assert.Equal(t, 2, data.Repository.PullRequests.Edges[1].Node.Number)
	})

	t.Run("Transport", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("f110/gh-test")
		repo.DefaultBranch("main")
		repo.PullRequests(
			NewPullRequest().Number(1).Title("Test PR").State(PullRequestStateOpen).Base("main").Head(nil, "feature"),
		)

		client := &http.Client{Transport: m.Transport()}
		body, _ := json.Marshal(graphqlRequest{
			Query: `{ repository(owner: "f110", name: "gh-test") { pullRequest(number: 1) { title } } }`,
		})
		httpResp, err := client.Post("https://api.github.com/graphql", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer httpResp.Body.Close()

		var resp graphqlResponse
		require.NoError(t, json.NewDecoder(httpResp.Body).Decode(&resp))
		require.Empty(t, resp.Errors)

		var data struct {
			Repository struct {
				PullRequest struct{ Title string }
			}
		}
		require.NoError(t, json.Unmarshal(resp.Data, &data))
		assert.Equal(t, "Test PR", data.Repository.PullRequest.Title)
	})
}
