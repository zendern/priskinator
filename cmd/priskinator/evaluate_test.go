package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zendern/priskinator/internal/policy"
	"github.com/zendern/priskinator/internal/scoring"
	"github.com/zendern/priskinator/internal/verdict"
)

var update = flag.Bool("update", false, "rewrite golden files")

// repo is a scratch git repository for evaluate tests.
type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("commit", "-q", "--allow-empty", "-m", "root")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null",
	}, args...)...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *repo) write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) symlink(target, path string) {
	r.t.Helper()
	if err := os.Symlink(target, filepath.Join(r.dir, path)); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) commit(msg string) string {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "org.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// golden compares got with testdata/evaluate/<name>, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "evaluate", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch (run go test -update to accept)\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func runCLI(args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func TestEvaluateGolden(t *testing.T) {
	docs := newRepo(t)
	docs.write("README.md", "# readme\n")
	docs.write("gen/api.go", "package gen\n")
	docs.write(".gitattributes", "gen/** linguist-generated\n")
	docsBase := docs.commit("base")
	docs.write("README.md", "# readme\n\nMore words.\n")
	docs.write("gen/api.go", "package gen\n\nvar X = 1\n")
	docsHead := docs.commit("docs")

	loose := newRepo(t)
	loose.write(".priskinator/policy.yaml", "version: 1\nlimits:\n  max_changed_lines: 4000\n")
	looseBase := loose.commit("base")
	loose.write(".priskinator/policy.yaml", "version: 1\nlimits:\n  max_changed_lines: 9000\n")
	looseHead := loose.commit("loosen")

	org := writeFile(t, "version: 99\n")

	tests := []struct {
		name string
		args []string
		code int
	}{
		{"docs.txt", []string{"--repo", docs.dir, "--base", docsBase, "--head", docsHead}, exitOK},
		{"docs.json", []string{"--repo", docs.dir, "--base", docsBase, "--head", docsHead, "--format", "json"}, exitOK},
		{"loosening.txt", []string{"--repo", loose.dir, "--base", looseBase, "--head", looseHead}, exitOK},
		{"loosening.json", []string{"--repo", loose.dir, "--base", looseBase, "--head", looseHead, "--format", "json"}, exitOK},
		{"org-invalid.txt", []string{"--repo", docs.dir, "--base", docsBase, "--head", docsHead, "--org-policy", org}, exitError},
		{"org-invalid.json", []string{"--repo", docs.dir, "--base", docsBase, "--head", docsHead, "--org-policy", org, "--format", "json"}, exitError},
		{"docs.md", []string{"--repo", docs.dir, "--base", docsBase, "--head", docsHead, "--format", "markdown"}, exitOK},
		{"loosening.md", []string{"--repo", loose.dir, "--base", looseBase, "--head", looseHead, "--format", "markdown"}, exitOK},
		{"org-invalid.md", []string{"--repo", docs.dir, "--base", docsBase, "--head", docsHead, "--org-policy", org, "--format", "markdown"}, exitError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runCLI(append([]string{"evaluate"}, tt.args...)...)
			if code != tt.code {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tt.code, stderr)
			}
			if strings.HasSuffix(tt.name, ".json") && !json.Valid([]byte(stdout)) {
				t.Errorf("output is not JSON: %s", stdout)
			}
			golden(t, tt.name, stdout)
		})
	}
}

func TestEvaluateUsage(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--base", "main"},
		{"--head", "main"},
		{"--base", "main", "--head", "main", "--format", "yaml"},
		{"--base", "main", "--head", "main", "extra"},
		{"--bogus"},
	} {
		stdout, stderr, code := runCLI(append([]string{"evaluate"}, args...)...)
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "usage: priskinator evaluate") {
			t.Errorf("evaluate %v = %d, stdout %q, stderr %q; want usage", args, code, stdout, stderr)
		}
	}
}

func TestEvaluateFailsClosed(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	base := r.commit("base")
	r.write("a.txt", "b\n")
	head := r.commit("head")

	bigOrg := writeFile(t, strings.Repeat("#", policy.MaxPolicyBytes+1))

	tests := []struct {
		name string
		opts evaluateOptions
		want string
	}{
		{"bad base", evaluateOptions{repo: r.dir, base: "nope", head: head}, "resolve nope"},
		{"bad head", evaluateOptions{repo: r.dir, base: base, head: "nope"}, "resolve nope"},
		{"missing git", evaluateOptions{repo: r.dir, base: base, head: head, git: filepath.Join(t.TempDir(), "git")}, ""},
		{"missing org", evaluateOptions{repo: r.dir, base: base, head: head, orgPolicy: filepath.Join(t.TempDir(), "nope.yaml")}, "org baseline"},
		{"org is a directory", evaluateOptions{repo: r.dir, base: base, head: head, orgPolicy: t.TempDir()}, "org baseline"},
		{"oversize org", evaluateOptions{repo: r.dir, base: base, head: head, orgPolicy: bigOrg}, "over the policy size limit"},
		{"diff fails", evaluateOptions{repo: r.dir, base: base, head: head, git: fakeGit(t, "--raw")}, "list changes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := evaluate(context.Background(), tt.opts); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEvaluateRejectsNonRegularFiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		head bool
	}{
		{"base policy", policyPath, false},
		{"base attributes", attributesPath, false},
		{"head policy", policyPath, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			r.write("target", "version: 1\n")
			if err := os.MkdirAll(filepath.Join(r.dir, ".priskinator"), 0o755); err != nil {
				t.Fatal(err)
			}
			if !tc.head {
				r.symlink(filepath.Join("..", "target"), tc.path)
			}
			base := r.commit("base")
			if tc.head {
				r.symlink(filepath.Join("..", "target"), tc.path)
			}
			r.write("a.txt", "a\n")
			head := r.commit("head")
			_, err := evaluate(context.Background(), evaluateOptions{repo: r.dir, base: base, head: head})
			if err == nil || !strings.Contains(err.Error(), "not a regular file") {
				t.Errorf("error = %v, want a not-regular error", err)
			}
		})
	}
}

func TestProposedWeakenings(t *testing.T) {
	loose := []byte("version: 1\nlimits:\n  max_changed_lines: 9000\n")
	head := func(b []byte) func() ([]byte, error) { return func() ([]byte, error) { return b, nil } }

	got, err := proposedWeakenings(nil, nil, head(loose))
	if err != nil || len(got) != 1 || got[0].Path != "/limits/max_changed_lines" {
		t.Errorf("new loosening = %+v, %v", got, err)
	}
	if got, err := proposedWeakenings(nil, loose, head(loose)); err != nil || got != nil {
		t.Errorf("unchanged policy = %+v, %v; want none", got, err)
	}
	if got, err := proposedWeakenings(nil, loose, head(nil)); err != nil || got != nil {
		t.Errorf("deleted policy = %+v, %v; want none", got, err)
	}
	if _, err := proposedWeakenings([]byte("version: 99\n"), nil, head(loose)); err == nil {
		t.Error("an invalid org baseline was accepted")
	}
	boom := errors.New("boom")
	if _, err := proposedWeakenings(nil, nil, func() ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("read error = %v, want boom", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestEvaluateWriteFailure(t *testing.T) {
	r := newRepo(t)
	head := r.commit("head")
	for _, format := range []string{"human", "json", "markdown"} {
		var stderr bytes.Buffer
		code := run([]string{"evaluate", "--repo", r.dir, "--base", head, "--head", head, "--format", format}, failingWriter{}, &stderr)
		if code != exitError || !strings.Contains(stderr.String(), "write verdict: disk full") {
			t.Errorf("%s: exit %d, stderr %q", format, code, stderr.String())
		}
	}
}

func TestHumanShowsNotes(t *testing.T) {
	v := verdict.Verdict{Score: &scoring.Result{Notes: []string{"policy sets no scoring thresholds"}}}
	if got := human(v); !strings.Contains(got, "  note: policy sets no scoring thresholds\n") {
		t.Errorf("human() = %q, want the note", got)
	}
}

func TestWithEmptyListsKeepsValues(t *testing.T) {
	v := verdict.Verdict{Triggers: []string{"a"}}
	if got := withEmptyLists(v); len(got.Triggers) != 1 || got.Score != nil || got.Reasons == nil {
		t.Errorf("withEmptyLists = %+v", got)
	}
}

// fakeGit returns a git wrapper that fails any call whose arguments include match.
func fakeGit(t *testing.T, match string) string {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "git")
	body := "#!/bin/sh\ncase \" $* \" in *\" " + match + " \"*) echo injected failure >&2; exit 1;; esac\nexec " + real + " \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}
