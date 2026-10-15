package changeset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCommit(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	head := r.commit("add a")
	r.git("tag", "v1")
	ctx := context.Background()

	for _, rev := range []string{"main", "v1", "HEAD", head[:7], head} {
		got, err := ResolveCommit(ctx, r.dir, rev, Options{})
		if err != nil || got != head {
			t.Errorf("ResolveCommit(%q) = %q, %v; want %s", rev, got, err, head)
		}
	}
	for _, rev := range []string{"", "-p", "--output=x", "nope"} {
		if _, err := ResolveCommit(ctx, r.dir, rev, Options{}); err == nil {
			t.Errorf("ResolveCommit(%q) succeeded", rev)
		}
	}
	tree := r.git("rev-parse", "HEAD^{tree}")
	if _, err := ResolveCommit(ctx, r.dir, tree, Options{}); err == nil {
		t.Error("a tree resolved as a commit")
	}
	if _, err := ResolveCommit(ctx, r.dir, "main", Options{Git: filepath.Join(t.TempDir(), "missing-git")}); err == nil {
		t.Error("ResolveCommit with a missing git succeeded")
	}
}

func TestReadFile(t *testing.T) {
	r := newRepo(t)
	r.write(".priskinator/policy.yaml", "version: 1\n")
	r.write("big.txt", strings.Repeat("x", 100))
	r.write("run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(r.dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("big.txt", filepath.Join(r.dir, "link")); err != nil {
		t.Fatal(err)
	}
	commit := r.commit("files")
	ctx := context.Background()

	data, ok, err := ReadFile(ctx, r.dir, commit, ".priskinator/policy.yaml", 1<<20, Options{})
	if err != nil || !ok || string(data) != "version: 1\n" {
		t.Errorf("policy = %q, %v, %v", data, ok, err)
	}
	if data, ok, err := ReadFile(ctx, r.dir, commit, "run.sh", 100, Options{}); err != nil || !ok || string(data) != "#!/bin/sh\n" {
		t.Errorf("executable = %q, %v, %v", data, ok, err)
	}
	if _, ok, err := ReadFile(ctx, r.dir, commit, "big.txt", 100, Options{}); err != nil || !ok {
		t.Errorf("exactly at the limit: %v, %v", ok, err)
	}

	for _, missing := range []string{"nope.yaml", ".priskinator/other.yaml", "policy.yaml"} {
		data, ok, err := ReadFile(ctx, r.dir, commit, missing, 1<<20, Options{})
		if err != nil || ok || data != nil {
			t.Errorf("%s: %q, %v, %v; want a miss", missing, data, ok, err)
		}
	}

	if _, _, err := ReadFile(ctx, r.dir, commit, "big.txt", 99, Options{}); err == nil || !strings.Contains(err.Error(), "100 bytes, over the 99-byte limit") {
		t.Errorf("oversize: %v", err)
	}
	for _, p := range []string{"link", ".priskinator"} {
		if _, _, err := ReadFile(ctx, r.dir, commit, p, 1<<20, Options{}); !errors.Is(err, ErrNotRegular) {
			t.Errorf("%s: err = %v, want ErrNotRegular", p, err)
		}
	}
	if _, _, err := ReadFile(ctx, r.dir, "main", "big.txt", 1<<20, Options{}); err == nil {
		t.Error("a branch name was accepted as a commit ID")
	}
	if _, _, err := ReadFile(ctx, r.dir, commit, "big.txt", 1<<20, Options{Git: filepath.Join(t.TempDir(), "missing-git")}); err == nil {
		t.Error("ReadFile with a missing git succeeded")
	}
}

func TestReadFileStepFailures(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	commit := r.commit("a")
	ctx := context.Background()

	tests := []struct{ match, action, want string }{
		{"cat-file -t", injectFailure, "revision"},
		{"ls-tree", injectFailure, "look up a.txt"},
		{"cat-file -s", injectFailure, "size of a.txt"},
		{"cat-file -s", "echo notanumber; exit 0", "size of a.txt"},
		{"cat-file blob", injectFailure, "read a.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.match+" "+tt.want, func(t *testing.T) {
			_, _, err := ReadFile(ctx, r.dir, commit, "a.txt", 100, Options{Git: fakeGit(t, tt.match, tt.action)})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
