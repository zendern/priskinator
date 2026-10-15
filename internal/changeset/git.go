package changeset

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Options configures FromGit.
type Options struct {
	// Git is the git binary. Defaults to "git" on PATH.
	Git string
}

// sniffBytes is how much of a blob is inspected, matching git's own binary heuristic.
const sniffBytes = 8000

var objectID = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// FromGit builds the change set between two commits in the repository at dir.
//
// base and head must be full commit IDs. The head is read only as git objects:
// nothing configured by the repository runs. Git is called through plumbing
// commands with a scrubbed environment, external diff and textconv disabled,
// hooks and fsmonitor off, and attributes read from an empty tree, so neither
// .git/config nor a .gitattributes in the PR can change or hide the diff.
func FromGit(ctx context.Context, dir, base, head string, opts Options) (*ChangeSet, error) {
	g, err := newGit(dir, opts)
	if err != nil {
		return nil, err
	}
	for _, rev := range []string{base, head} {
		if err := g.verifyCommit(ctx, rev); err != nil {
			return nil, err
		}
	}

	raw, err := g.run(ctx, nil, g.diffArgs("--raw", "-z", base, head)...)
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}
	files, err := parseRaw(raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw diff: %w", err)
	}

	patch, err := g.run(ctx, nil, g.diffArgs("-p", "-U0", "--submodule=short", "--src-prefix=a/", "--dst-prefix=b/", base, head)...)
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}
	if err := attachPatches(files, patch); err != nil {
		return nil, fmt.Errorf("parse patch: %w", err)
	}

	if err := g.inspectBlobs(ctx, files); err != nil {
		return nil, err
	}
	for i := range files {
		dropNonContentHunks(&files[i])
	}
	return &ChangeSet{Base: base, Head: head, Files: files}, nil
}

// dropNonContentHunks removes lines that are not file content: everything for
// binary files, and the "Subproject commit" lines git prints for a submodule side.
func dropNonContentHunks(f *File) {
	if f.Binary {
		f.Hunks = nil
		return
	}
	var kept []Hunk
	for _, h := range f.Hunks {
		if f.OldMode == ModeSubmodule {
			h.Removed = nil
		}
		if f.NewMode == ModeSubmodule {
			h.Added = nil
		}
		if len(h.Removed) > 0 || len(h.Added) > 0 {
			kept = append(kept, h)
		}
	}
	f.Hunks = kept
}

type gitRunner struct {
	bin       string
	dir       string
	env       []string
	emptyTree string
}

func newGit(dir string, opts Options) (*gitRunner, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	bin := opts.Git
	if bin == "" {
		bin = "git"
	}
	g := &gitRunner{
		bin: bin,
		dir: abs,
		// Only what git needs; drops GIT_DIR, GIT_EXTERNAL_DIFF, GIT_CONFIG_*,
		// pagers, and anything else the caller's environment might carry.
		env: []string{
			"PATH=" + os.Getenv("PATH"),
			"LC_ALL=C",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL=" + os.DevNull,
			"GIT_TERMINAL_PROMPT=0",
			"GIT_OPTIONAL_LOCKS=0",
			"GIT_NO_REPLACE_OBJECTS=1",
			"GIT_PAGER=cat",
			"PAGER=cat",
		},
	}
	out, err := g.run(context.Background(), strings.NewReader(""), "hash-object", "-t", "tree", "--stdin")
	if err != nil {
		return nil, fmt.Errorf("compute empty tree: %w", err)
	}
	g.emptyTree = strings.TrimSpace(string(out))
	return g, nil
}

// run executes git with the hardening options every call shares.
func (g *gitRunner) run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	base := []string{
		"--no-pager",
		"-c", "safe.directory=" + g.dir,
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "core.fsmonitor=false",
		"-c", "core.attributesFile=" + os.DevNull,
		"-c", "core.quotePath=false",
		"-c", "diff.external=",
		"-c", "color.ui=never",
	}
	if g.emptyTree != "" {
		base = append(base, "--attr-source="+g.emptyTree)
	}
	cmd := exec.CommandContext(ctx, g.bin, append(base, args...)...) //nolint:gosec // fixed binary; args are validated object IDs and constant flags
	cmd.Dir = g.dir
	cmd.Env = g.env
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// diffArgs builds a diff-tree call. Plumbing ignores porcelain diff settings,
// and each flag below pins one that could still apply.
func (g *gitRunner) diffArgs(extra ...string) []string {
	args := []string{
		"diff-tree", "-r", "--no-commit-id", "--no-abbrev",
		"--no-ext-diff", "--no-textconv", "--no-color", "--no-relative",
		"--ignore-submodules=none", "-O" + os.DevNull,
		"--find-renames", "--find-copies",
	}
	return append(args, extra...)
}

func (g *gitRunner) verifyCommit(ctx context.Context, rev string) error {
	if !objectID.MatchString(rev) {
		return fmt.Errorf("revision %q is not a full commit ID", rev)
	}
	out, err := g.run(ctx, nil, "cat-file", "-t", rev)
	if err != nil {
		return fmt.Errorf("revision %s: %w", rev, err)
	}
	if t := strings.TrimSpace(string(out)); t != "commit" {
		return fmt.Errorf("revision %s is a %s, not a commit", rev, t)
	}
	return nil
}

// parseRaw reads `diff-tree --raw -z` output.
func parseRaw(raw []byte) ([]File, error) {
	fields := bytes.Split(raw, []byte{0})
	// bytes.Split always returns at least one field.
	if last := len(fields) - 1; len(fields[last]) == 0 {
		fields = fields[:last]
	}
	var files []File
	for i := 0; i < len(fields); {
		meta := string(fields[i])
		i++
		// :<old mode> <new mode> <old oid> <new oid> <status><score>
		parts := strings.Fields(strings.TrimPrefix(meta, ":"))
		if !strings.HasPrefix(meta, ":") || len(parts) != 5 || parts[4] == "" {
			return nil, fmt.Errorf("unexpected raw entry %q", meta)
		}
		f := File{OldMode: parts[0], NewMode: parts[1], OldOID: parts[2], NewOID: parts[3]}
		code, score := parts[4][0], parts[4][1:]
		paths := 1
		switch code {
		case 'A':
			f.Status = StatusAdded
		case 'M':
			f.Status = StatusModified
		case 'D':
			f.Status = StatusDeleted
		case 'T':
			f.Status = StatusTypeChanged
		case 'R':
			f.Status, paths = StatusRenamed, 2
		case 'C':
			f.Status, paths = StatusCopied, 2
		default:
			return nil, fmt.Errorf("unsupported status %q", parts[4])
		}
		if score != "" {
			s, err := strconv.Atoi(score)
			if err != nil {
				return nil, fmt.Errorf("bad score in %q", meta)
			}
			f.Similarity = s
		}
		if i+paths > len(fields) {
			return nil, fmt.Errorf("missing path for %q", meta)
		}
		if paths == 2 {
			f.OldPath = string(fields[i])
			f.Path = string(fields[i+1])
		} else {
			f.Path = string(fields[i])
		}
		i += paths
		if isZeroMode(f.OldMode) {
			f.OldMode, f.OldOID = "", ""
		}
		if isZeroMode(f.NewMode) {
			f.NewMode, f.NewOID = "", ""
		}
		files = append(files, f)
	}
	return files, nil
}

func isZeroMode(m string) bool { return strings.Trim(m, "0") == "" }

// inspectBlobs reads sizes and the first bytes of every blob to detect binary
// content, LFS pointers, and symlink targets, without running any filter.
func (g *gitRunner) inspectBlobs(ctx context.Context, files []File) error {
	var oids []string
	for _, f := range files {
		if f.OldOID != "" && f.OldMode != ModeSubmodule {
			oids = append(oids, f.OldOID)
		}
		if f.NewOID != "" && f.NewMode != ModeSubmodule {
			oids = append(oids, f.NewOID)
		}
	}
	if len(oids) == 0 {
		return nil
	}
	blobs, err := g.readBlobs(ctx, oids)
	if err != nil {
		return err
	}
	for i := range files {
		f := &files[i]
		// Submodule commits were never requested, so they are not in blobs.
		if b, ok := blobs[f.OldOID]; ok {
			f.OldSize = b.size
			f.Binary = f.Binary || isBinary(b.head)
			f.LFS = f.LFS || isLFSPointer(b)
		}
		if b, ok := blobs[f.NewOID]; ok {
			f.NewSize = b.size
			f.Binary = f.Binary || isBinary(b.head)
			f.LFS = f.LFS || isLFSPointer(b)
			if f.NewMode == ModeSymlink {
				f.SymlinkTarget = string(b.head)
			}
		}
	}
	return nil
}

type blob struct {
	size int64
	head []byte
}

func (g *gitRunner) readBlobs(ctx context.Context, oids []string) (map[string]blob, error) {
	cmd := exec.CommandContext(ctx, g.bin, "--no-pager", //nolint:gosec // fixed binary and constant args; object IDs go through stdin
		"-c", "safe.directory="+g.dir, "-c", "core.hooksPath="+os.DevNull, "-c", "core.fsmonitor=false",
		"cat-file", "--batch")
	cmd.Dir = g.dir
	cmd.Env = g.env
	cmd.Stdin = strings.NewReader(strings.Join(oids, "\n") + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git cat-file: %w", err)
	}
	blobs, readErr := parseBatch(bufio.NewReader(stdout), len(oids))
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, fmt.Errorf("read blobs: %w", readErr)
	}
	if waitErr != nil {
		return nil, fmt.Errorf("git cat-file: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return blobs, nil
}

func parseBatch(r *bufio.Reader, n int) (map[string]blob, error) {
	blobs := make(map[string]blob, n)
	for range n {
		header, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		parts := strings.Fields(header)
		if len(parts) == 2 && parts[1] == "missing" {
			return nil, fmt.Errorf("object %s is missing", parts[0])
		}
		if len(parts) != 3 || parts[1] != "blob" {
			return nil, fmt.Errorf("unexpected object header %q", strings.TrimSpace(header))
		}
		size, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("bad size in %q", strings.TrimSpace(header))
		}
		head := make([]byte, min(size, sniffBytes))
		if _, err := io.ReadFull(r, head); err != nil {
			return nil, err
		}
		if _, err := io.CopyN(io.Discard, r, size-int64(len(head))+1); err != nil { // rest plus trailing newline
			return nil, err
		}
		blobs[parts[0]] = blob{size: size, head: head}
	}
	if _, err := r.ReadByte(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected trailing output")
	}
	return blobs, nil
}

func isBinary(head []byte) bool { return bytes.IndexByte(head, 0) >= 0 }

// Git LFS pointer files are small text files with a fixed first line
// (https://github.com/git-lfs/git-lfs/blob/main/docs/spec.md).
func isLFSPointer(b blob) bool {
	if b.size >= 1024 {
		return false
	}
	return bytes.HasPrefix(b.head, []byte("version https://git-lfs.github.com/spec/v1\n")) ||
		bytes.HasPrefix(b.head, []byte("version https://hawser.github.com/spec/v1\n"))
}
