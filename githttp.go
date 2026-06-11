package githubmock

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	gittransport "github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/go-git/go-git/v5/storage/memory"
)

const uploadPackService = "git-upload-pack"

// GitHTTPHandler returns an http.Handler that serves repositories over the git
// smart HTTP protocol. It supports cloning and fetching (git-upload-pack).
// The handler is intended to be served on a different port than the GitHub API.
func (m *Mock) GitHTTPHandler() http.Handler {
	return &gitHTTPHandler{mock: m}
}

type gitHTTPHandler struct {
	mock *Mock
}

func (h *gitHTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	owner, name, rest, ok := splitGitPath(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	repo := h.mock.LookupRepository(fmt.Sprintf("%s/%s", owner, name))
	if repo == nil {
		http.NotFound(w, req)
		return
	}

	switch {
	case req.Method == http.MethodGet && rest == "info/refs":
		h.serveInfoRefs(w, req, repo)
	case req.Method == http.MethodPost && rest == uploadPackService:
		h.serveUploadPack(w, req, repo)
	default:
		http.NotFound(w, req)
	}
}

// splitGitPath splits a request path like "/octocat/example/info/refs" into the
// owner, repository name and the remaining service path. A trailing ".git" on
// the repository name is accepted and stripped.
func splitGitPath(p string) (owner, name, rest string, ok bool) {
	p = strings.TrimPrefix(p, "/")
	s := strings.SplitN(p, "/", 3)
	if len(s) < 3 {
		return "", "", "", false
	}
	owner, name, rest = s[0], s[1], s[2]
	name = strings.TrimSuffix(name, ".git")
	if owner == "" || name == "" || rest == "" {
		return "", "", "", false
	}
	return owner, name, rest, true
}

func (h *gitHTTPHandler) serveInfoRefs(w http.ResponseWriter, req *http.Request, repo *Repository) {
	if req.URL.Query().Get("service") != uploadPackService {
		// Only the smart protocol with git-upload-pack is supported.
		http.Error(w, "only git-upload-pack is supported", http.StatusForbidden)
		return
	}

	sess, err := h.uploadPackSession(repo)
	if err != nil {
		h.internalError(req.Context(), w, "failed to start upload-pack session", err)
		return
	}
	defer sess.Close()

	ar, err := sess.AdvertisedReferencesContext(req.Context())
	if err != nil {
		h.internalError(req.Context(), w, "failed to advertise references", err)
		return
	}
	// Prepend the service announcement required by the smart HTTP protocol.
	ar.Prefix = [][]byte{[]byte("# service=" + uploadPackService), pktline.Flush}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	if err := ar.Encode(w); err != nil {
		h.mock.Logger.ErrorContext(req.Context(), "failed to encode advertised references", slog.Any("err", err))
	}
}

func (h *gitHTTPHandler) serveUploadPack(w http.ResponseWriter, req *http.Request, repo *Repository) {
	upr := packp.NewUploadPackRequest()
	if err := upr.Decode(req.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sess, err := h.uploadPackSession(repo)
	if err != nil {
		h.internalError(req.Context(), w, "failed to start upload-pack session", err)
		return
	}
	defer sess.Close()

	resp, err := sess.UploadPack(req.Context(), upr)
	if err != nil {
		h.internalError(req.Context(), w, "failed to build pack", err)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	if err := resp.Encode(w); err != nil {
		h.mock.Logger.ErrorContext(req.Context(), "failed to encode upload-pack response", slog.Any("err", err))
	}
}

func (h *gitHTTPHandler) uploadPackSession(repo *Repository) (gittransport.UploadPackSession, error) {
	st, err := buildGitStorer(repo)
	if err != nil {
		return nil, err
	}
	ep, err := gittransport.NewEndpoint("http://localhost/")
	if err != nil {
		return nil, err
	}
	return server.NewServer(storerLoader{storer: st}).NewUploadPackSession(ep, nil)
}

func (h *gitHTTPHandler) internalError(ctx context.Context, w http.ResponseWriter, msg string, err error) {
	h.mock.Logger.ErrorContext(ctx, msg, slog.Any("err", err))
	http.Error(w, msg, http.StatusInternalServerError)
}

type storerLoader struct {
	storer storer.Storer
}

func (l storerLoader) Load(*gittransport.Endpoint) (storer.Storer, error) {
	return l.storer, nil
}

// buildGitStorer builds an in-memory git repository from the mock data of a
// Repository. Object hashes are computed by git from the content, so they do
// not match the (randomly generated) SHAs of the mock commits.
func buildGitStorer(repo *Repository) (storer.Storer, error) {
	repo.mu.Lock()
	commits := make([]*Commit, len(repo.commits))
	copy(commits, repo.commits)
	headCommit := repo.headCommit
	branch := repo.ghRepository.GetDefaultBranch()
	repo.mu.Unlock()

	st := memory.NewStorage()
	if len(commits) == 0 {
		return st, nil
	}

	ordered, err := topologicalCommits(commits)
	if err != nil {
		return nil, err
	}

	gitHash := make(map[*Commit]plumbing.Hash, len(ordered))
	for _, c := range ordered {
		treeHash, err := writeTree(st, c.files)
		if err != nil {
			return nil, err
		}

		var parents []plumbing.Hash
		for _, p := range c.parents {
			h, ok := gitHash[p]
			if !ok {
				return nil, fmt.Errorf("githubmock: parent commit is not built yet")
			}
			parents = append(parents, h)
		}

		when := time.Unix(0, 0).UTC()
		sig := object.Signature{Name: "githubmock", Email: "githubmock@localhost", When: when}
		commit := &object.Commit{
			Author:       sig,
			Committer:    sig,
			Message:      c.ghCommit.GetMessage(),
			TreeHash:     treeHash,
			ParentHashes: parents,
		}
		obj := st.NewEncodedObject()
		if err := commit.Encode(obj); err != nil {
			return nil, err
		}
		h, err := st.SetEncodedObject(obj)
		if err != nil {
			return nil, err
		}
		gitHash[c] = h
	}

	// The branch points at the tip of the history. Prefer an explicitly marked
	// head commit, otherwise use the newest leaf (a commit that is not a parent
	// of any other commit). The last entry in topological order is always such
	// a leaf.
	tip := headCommit
	if tip == nil {
		tip = ordered[len(ordered)-1]
	}
	if branch == "" {
		branch = "master"
	}
	refName := plumbing.NewBranchReferenceName(branch)
	if err := st.SetReference(plumbing.NewHashReference(refName, gitHash[tip])); err != nil {
		return nil, err
	}
	if err := st.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, refName)); err != nil {
		return nil, err
	}
	return st, nil
}

// topologicalCommits orders commits so that every parent appears before its
// children.
func topologicalCommits(commits []*Commit) ([]*Commit, error) {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[*Commit]int, len(commits))
	known := make(map[*Commit]struct{}, len(commits))
	for _, c := range commits {
		known[c] = struct{}{}
	}

	var ordered []*Commit
	var visit func(c *Commit) error
	visit = func(c *Commit) error {
		switch state[c] {
		case visiting:
			return fmt.Errorf("githubmock: commit graph has a cycle")
		case visited:
			return nil
		}
		state[c] = visiting
		for _, p := range c.parents {
			if _, ok := known[p]; !ok {
				continue
			}
			if err := visit(p); err != nil {
				return err
			}
		}
		state[c] = visited
		ordered = append(ordered, c)
		return nil
	}
	for _, c := range commits {
		if err := visit(c); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// treeNode is a node in the directory tree built from a flat list of files.
type treeNode struct {
	dirs  map[string]*treeNode
	files map[string][]byte
}

func newTreeNode() *treeNode {
	return &treeNode{dirs: make(map[string]*treeNode), files: make(map[string][]byte)}
}

func (n *treeNode) dir(name string) *treeNode {
	if c, ok := n.dirs[name]; ok {
		return c
	}
	c := newTreeNode()
	n.dirs[name] = c
	return c
}

// writeTree writes the blobs and tree objects for a commit's files and returns
// the hash of the root tree.
func writeTree(st storer.Storer, files []*File) (plumbing.Hash, error) {
	root := newTreeNode()
	for _, f := range files {
		if f.mode != fileTypeRegular || f.Name == "" {
			continue
		}
		parts := strings.Split(f.Name, "/")
		node := root
		for _, d := range parts[:len(parts)-1] {
			node = node.dir(d)
		}
		node.files[parts[len(parts)-1]] = f.Body
	}
	return writeTreeNode(st, root)
}

func writeTreeNode(st storer.Storer, node *treeNode) (plumbing.Hash, error) {
	var entries []object.TreeEntry
	for name, body := range node.files {
		blobHash, err := writeBlob(st, body)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Regular, Hash: blobHash})
	}
	for name, child := range node.dirs {
		childHash, err := writeTreeNode(st, child)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Dir, Hash: childHash})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	tree := &object.Tree{Entries: entries}
	obj := st.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return st.SetEncodedObject(obj)
}

func writeBlob(st storer.Storer, body []byte) (plumbing.Hash, error) {
	obj := st.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(body)))
	wc, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := wc.Write(body); err != nil {
		return plumbing.ZeroHash, err
	}
	if err := wc.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	return st.SetEncodedObject(obj)
}
