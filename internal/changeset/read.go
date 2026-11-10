package changeset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ResolveCommit returns the full ID of the commit rev names, such as a branch,
// tag, or abbreviated ID, in the repository at dir. --end-of-options keeps a
// revision that starts with "-" from being read as an option.
func ResolveCommit(ctx context.Context, dir, rev string, opts Options) (string, error) {
	g, err := newGit(ctx, dir, opts)
	if err != nil {
		return "", err
	}
	out, err := g.run(ctx, nil, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", rev, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ErrNotRegular is returned for a path that exists at a commit but is not a
// regular file, such as a symlink, a submodule, or a directory.
var ErrNotRegular = errors.New("not a regular file")

// ReadFile returns the content of path at commit, read from git's objects, and
// false when the path does not exist there. A path that is not a regular file,
// or is larger than limit bytes, is an error rather than a miss, so a symlinked
// or oversized policy can't pass as no policy.
func ReadFile(ctx context.Context, dir, commit, path string, limit int64, opts Options) ([]byte, bool, error) {
	g, err := newGit(ctx, dir, opts)
	if err != nil {
		return nil, false, err
	}
	if err := g.verifyCommit(ctx, commit); err != nil {
		return nil, false, err
	}
	out, err := g.run(ctx, nil, "ls-tree", "-z", "--full-tree", commit, "--", path)
	if err != nil {
		return nil, false, fmt.Errorf("look up %s: %w", path, err)
	}
	entry, _, _ := bytes.Cut(out, []byte{0})
	meta, name, _ := strings.Cut(string(entry), "\t")
	if name != path {
		return nil, false, nil
	}
	fields := strings.Fields(meta)
	if len(fields) != 3 || (fields[0] != ModeRegular && fields[0] != ModeExecutable) {
		return nil, false, fmt.Errorf("%s at %s: %w (%s)", path, commit, ErrNotRegular, meta)
	}
	oid := fields[2]
	sizeOut, err := g.run(ctx, nil, "cat-file", "-s", oid)
	if err != nil {
		return nil, false, fmt.Errorf("size of %s: %w", path, err)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOut)), 10, 64)
	if err != nil {
		return nil, false, fmt.Errorf("size of %s: %w", path, err)
	}
	if size > limit {
		return nil, false, fmt.Errorf("%s at %s is %d bytes, over the %d-byte limit", path, commit, size, limit)
	}
	data, err := g.run(ctx, nil, "cat-file", "blob", oid)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	return data, true, nil
}
