package githubmock

import (
	"net/http/httptest"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHTTPHandler(t *testing.T) {
	t.Run("Clone", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("octocat/example")
		repo.DefaultBranch("main")
		root := NewCommit().Files(&File{Name: "README.md", Body: []byte("hello\n")})
		child := NewCommit().
			Files(&File{Name: "README.md", Body: []byte("hello\n")}, &File{Name: "dir/main.go", Body: []byte("package main\n")}).
			Parents(root).
			IsHead()
		require.NoError(t, repo.Commits(root))
		require.NoError(t, repo.Commits(child))

		svr := httptest.NewServer(m.GitHTTPHandler())
		t.Cleanup(svr.Close)

		cloned, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
			URL: svr.URL + "/octocat/example",
		})
		require.NoError(t, err)

		head, err := cloned.Head()
		require.NoError(t, err)
		assert.Equal(t, plumbing.NewBranchReferenceName("main"), head.Name())

		// The whole history must be transferred.
		commitIter, err := cloned.Log(&git.LogOptions{From: head.Hash()})
		require.NoError(t, err)
		var count int
		for {
			_, err := commitIter.Next()
			if err != nil {
				break
			}
			count++
		}
		assert.Equal(t, 2, count)

		// The file contents must be readable from the transferred objects.
		wt, err := cloned.Worktree()
		require.NoError(t, err)
		f, err := wt.Filesystem.Open("dir/main.go")
		require.NoError(t, err)
		defer f.Close()
		buf := make([]byte, 32)
		n, _ := f.Read(buf)
		assert.Equal(t, "package main\n", string(buf[:n]))
	})

	t.Run("Fetch", func(t *testing.T) {
		m := NewMock()
		repo := m.Repository("octocat/example")
		repo.DefaultBranch("main")
		root := NewCommit().Files(&File{Name: "README.md", Body: []byte("hello\n")}).IsHead()
		require.NoError(t, repo.Commits(root))

		svr := httptest.NewServer(m.GitHTTPHandler())
		t.Cleanup(svr.Close)

		cloned, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
			URL: svr.URL + "/octocat/example",
		})
		require.NoError(t, err)

		err = cloned.Fetch(&git.FetchOptions{})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			t.Fatalf("fetch failed: %v", err)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		m := NewMock()
		svr := httptest.NewServer(m.GitHTTPHandler())
		t.Cleanup(svr.Close)

		_, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
			URL: svr.URL + "/octocat/missing",
		})
		require.Error(t, err)
	})
}
