package changeset

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestKind(t *testing.T) {
	tests := []struct {
		name string
		file File
		want Kind
	}{
		{"regular", File{NewMode: ModeRegular}, KindRegular},
		{"executable", File{NewMode: ModeExecutable}, KindExecutable},
		{"symlink", File{NewMode: ModeSymlink}, KindSymlink},
		{"submodule", File{NewMode: ModeSubmodule}, KindSubmodule},
		{"unknown mode", File{NewMode: "040000"}, KindUnknown},
		{"deletion uses the base mode", File{OldMode: ModeExecutable}, KindExecutable},
		{"head mode wins over base", File{OldMode: ModeRegular, NewMode: ModeSymlink}, KindSymlink},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.file.Kind(); got != tt.want {
				t.Errorf("Kind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPaths(t *testing.T) {
	tests := []struct {
		name string
		file File
		want []string
	}{
		{"plain", File{Path: "a.go"}, []string{"a.go"}},
		{"rename", File{Path: "b.go", OldPath: "a.go"}, []string{"b.go", "a.go"}},
		{"same old path", File{Path: "a.go", OldPath: "a.go"}, []string{"a.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.file.Paths(); !slices.Equal(got, tt.want) {
				t.Errorf("Paths() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLineCounts(t *testing.T) {
	f := File{Hunks: []Hunk{
		{Added: []Line{{1, "a"}, {2, "b"}}, Removed: []Line{{1, "x"}}},
		{Added: []Line{{9, "c"}}, Removed: []Line{{7, "y"}, {8, "z"}, {9, "w"}}},
	}}
	if got := f.AddedLines(); got != 3 {
		t.Errorf("AddedLines() = %d, want 3", got)
	}
	if got := f.RemovedLines(); got != 4 {
		t.Errorf("RemovedLines() = %d, want 4", got)
	}
}

const (
	oidA = "1111111111111111111111111111111111111111"
	oidB = "2222222222222222222222222222222222222222"
	oidZ = "0000000000000000000000000000000000000000"
)

func rawEntry(meta string, paths ...string) string {
	return meta + "\x00" + strings.Join(paths, "\x00") + "\x00"
}

func TestModeChanged(t *testing.T) {
	tests := []struct {
		old, new string
		want     bool
	}{
		{ModeRegular, ModeExecutable, true},
		{ModeRegular, ModeRegular, false},
		{"", ModeRegular, false},
		{ModeRegular, "", false},
	}
	for _, tt := range tests {
		if got := (File{OldMode: tt.old, NewMode: tt.new}).ModeChanged(); got != tt.want {
			t.Errorf("ModeChanged(%q, %q) = %v, want %v", tt.old, tt.new, got, tt.want)
		}
	}
}

func TestParseRaw(t *testing.T) {
	raw := rawEntry(":100644 100755 "+oidA+" "+oidB+" M", "run.sh") +
		rawEntry(":000000 100644 "+oidZ+" "+oidB+" A", "new.go") +
		rawEntry(":100644 000000 "+oidA+" "+oidZ+" D", "old.go") +
		rawEntry(":100644 120000 "+oidA+" "+oidB+" T", "link") +
		rawEntry(":100644 100644 "+oidA+" "+oidB+" R087", "from.go", "to.go") +
		rawEntry(":100644 100644 "+oidA+" "+oidB+" C100", "src.go", "dst.go")
	files, err := parseRaw([]byte(raw))
	if err != nil {
		t.Fatalf("parseRaw: %v", err)
	}
	want := []File{
		{Path: "run.sh", Status: StatusModified, OldMode: ModeRegular, NewMode: ModeExecutable, OldOID: oidA, NewOID: oidB},
		{Path: "new.go", Status: StatusAdded, NewMode: ModeRegular, NewOID: oidB},
		{Path: "old.go", Status: StatusDeleted, OldMode: ModeRegular, OldOID: oidA},
		{Path: "link", Status: StatusTypeChanged, OldMode: ModeRegular, NewMode: ModeSymlink, OldOID: oidA, NewOID: oidB},
		{Path: "to.go", OldPath: "from.go", Status: StatusRenamed, OldMode: ModeRegular, NewMode: ModeRegular, OldOID: oidA, NewOID: oidB, Similarity: 87},
		{Path: "dst.go", OldPath: "src.go", Status: StatusCopied, OldMode: ModeRegular, NewMode: ModeRegular, OldOID: oidA, NewOID: oidB, Similarity: 100},
	}
	if len(files) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(files), len(want), files)
	}
	for i := range want {
		if !filesEqual(files[i], want[i]) {
			t.Errorf("file %d = %+v, want %+v", i, files[i], want[i])
		}
	}
}

func filesEqual(a, b File) bool {
	return a.Path == b.Path && a.OldPath == b.OldPath && a.Status == b.Status &&
		a.OldMode == b.OldMode && a.NewMode == b.NewMode && a.OldOID == b.OldOID &&
		a.NewOID == b.NewOID && a.Similarity == b.Similarity
}

func TestParseRawEmpty(t *testing.T) {
	for _, raw := range []string{""} {
		files, err := parseRaw([]byte(raw))
		if err != nil || len(files) != 0 {
			t.Errorf("parseRaw(%q) = %v, %v; want no files", raw, files, err)
		}
	}
}

func TestParseRawErrors(t *testing.T) {
	tests := []struct {
		name, raw, want string
	}{
		{"no colon", rawEntry("100644 100644 "+oidA+" "+oidB+" M", "a"), "unexpected raw entry"},
		{"too few fields", rawEntry(":100644 100644 "+oidA+" M", "a"), "unexpected raw entry"},
		{"unknown status", rawEntry(":100644 100644 "+oidA+" "+oidB+" X", "a"), `unsupported status "X"`},
		{"bad score", rawEntry(":100644 100644 "+oidA+" "+oidB+" R8x", "a", "b"), "bad score"},
		{"missing path", ":100644 100644 " + oidA + " " + oidB + " M\x00", "missing path"},
		{"rename missing new path", ":100644 100644 " + oidA + " " + oidB + " R090\x00a\x00", "missing path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRaw([]byte(tt.raw))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("parseRaw error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestIsZeroMode(t *testing.T) {
	for m, want := range map[string]bool{"000000": true, "": true, "100644": false, "160000": false} {
		if got := isZeroMode(m); got != want {
			t.Errorf("isZeroMode(%q) = %v, want %v", m, got, want)
		}
	}
}

func batch(entries ...string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(strings.Join(entries, "")))
}

func TestParseBatch(t *testing.T) {
	big := strings.Repeat("x", sniffBytes+5)
	blobs, err := parseBatch(batch(
		oidA+" blob 5\nhello\n",
		oidB+" blob 0\n\n",
		oidZ+" blob "+strconv.Itoa(len(big))+"\n"+big+"\n",
	), 3)
	if err != nil {
		t.Fatalf("parseBatch: %v", err)
	}
	if b := blobs[oidA]; b.size != 5 || string(b.head) != "hello" {
		t.Errorf("blob A = %d %q", b.size, b.head)
	}
	if b, ok := blobs[oidB]; !ok || b.size != 0 || len(b.head) != 0 {
		t.Errorf("blob B = %v %d %q", ok, b.size, b.head)
	}
	if b := blobs[oidZ]; b.size != int64(len(big)) || len(b.head) != sniffBytes {
		t.Errorf("big blob size %d, head %d bytes; want %d, %d", b.size, len(b.head), len(big), sniffBytes)
	}
}

func TestParseBatchErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"missing object", oidA + " missing\n", 1, "is missing"},
		{"not a blob", oidA + " tree 5\nhello\n", 1, "unexpected object header"},
		{"short header", oidA + " blob\n", 1, "unexpected object header"},
		{"bad size", oidA + " blob five\nhello\n", 1, "bad size"},
		{"negative size", oidA + " blob -1\n", 1, "bad size"},
		{"no header", "", 1, "EOF"},
		{"truncated content", oidA + " blob 5\nhel", 1, "EOF"},
		{"missing trailing newline", oidA + " blob 5\nhello", 1, "EOF"},
		{"trailing output", oidA + " blob 5\nhello\nmore", 1, "unexpected trailing output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseBatch(batch(tt.input), tt.n)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("parseBatch error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestIsLFSPointer(t *testing.T) {
	spec := "version https://git-lfs.github.com/spec/v1\noid sha256:abc\nsize 12\n"
	hawser := "version https://hawser.github.com/spec/v1\noid sha256:abc\n"
	tests := []struct {
		name string
		b    blob
		want bool
	}{
		{"git-lfs pointer", blob{size: int64(len(spec)), head: []byte(spec)}, true},
		{"hawser pointer", blob{size: int64(len(hawser)), head: []byte(hawser)}, true},
		{"just under the size limit", blob{size: 1023, head: []byte(spec)}, true},
		{"at the size limit", blob{size: 1024, head: []byte(spec)}, false},
		{"plain text", blob{size: 5, head: []byte("hello")}, false},
		{"prefix without newline", blob{size: 42, head: []byte("version https://git-lfs.github.com/spec/v1")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLFSPointer(tt.b); got != tt.want {
				t.Errorf("isLFSPointer = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		in   string
		want Hunk
	}{
		{"@@ -3 +3 @@", Hunk{OldStart: 3, OldLines: 1, NewStart: 3, NewLines: 1}},
		{"@@ -3,2 +4,5 @@ func f() {", Hunk{OldStart: 3, OldLines: 2, NewStart: 4, NewLines: 5}},
		{"@@ -0,0 +1,3 @@", Hunk{OldStart: 1, OldLines: 0, NewStart: 1, NewLines: 3}},
		{"@@ -7,2 +6,0 @@", Hunk{OldStart: 7, OldLines: 2, NewStart: 7, NewLines: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseHunkHeader(tt.in)
			if err != nil {
				t.Fatalf("parseHunkHeader: %v", err)
			}
			if got.OldStart != tt.want.OldStart || got.OldLines != tt.want.OldLines ||
				got.NewStart != tt.want.NewStart || got.NewLines != tt.want.NewLines {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseHunkHeaderErrors(t *testing.T) {
	for _, in := range []string{
		"@@ 3 +3 @@",
		"@@ -3 +3",
		"@@ -3 3 @@",
		"@@ -x +3 @@",
		"@@ -3 +y @@",
		"@@ -3,x +3 @@",
		"@@ -3 +3,y @@",
		"@@ --1 +3 @@",
		"@@ -3 +3,-2 @@",
	} {
		if _, err := parseHunkHeader(in); err == nil || !strings.Contains(err.Error(), "bad hunk header") {
			t.Errorf("parseHunkHeader(%q) error = %v, want a bad hunk header error", in, err)
		}
	}
}

func TestQuotePath(t *testing.T) {
	tests := map[string]string{
		"a/plain/path.go":   "a/plain/path.go",
		"a/ünïcode spaced":  "a/ünïcode spaced",
		`a/q"uote`:          `"a/q\"uote"`,
		`a/back\slash`:      `"a/back\\slash"`,
		"a/\a\b\t\n\v\f\r":  `"a/\a\b\t\n\v\f\r"`,
		"a/\x01ctl\x7fdel":  `"a/\001ctl\177del"`,
		"a/\x1fus":          `"a/\037us"`,
		"a/tilde~ok":        "a/tilde~ok",
		"a/\"mixed\x00\x20": `"a/\"mixed\000 "`,
	}
	for in, want := range tests {
		if got := quotePath(in); got != want {
			t.Errorf("quotePath(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestSplitBlocksErrors(t *testing.T) {
	tests := []struct {
		name, patch, want string
	}{
		{"no header", "junk\n", "line 1: expected diff header"},
		{"bad hunk header", "diff --git a/x b/x\n@@ -x +1 @@\n", "line 2: bad hunk header"},
		{"truncated hunk", "diff --git a/x b/x\n@@ -1,2 +1,2 @@\n-a\n", "truncated"},
		{"bad hunk line", "diff --git a/x b/x\n@@ -1 +1 @@\n?a\n+b\n", "unexpected hunk line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := splitBlocks([]byte(tt.patch))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("splitBlocks error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestSplitBlocksContextAndBinary(t *testing.T) {
	patch := "diff --git a/x b/x\n" +
		"index 1..2 100644\n" +
		"@@ -1,3 +1,3 @@\n" +
		" keep\n" +
		"-old\n" +
		"+new\n" +
		" keep\n" +
		"\\ No newline at end of file\n" +
		"diff --git a/img b/img\n" +
		"GIT binary patch\n" +
		"diff --git a/bin b/bin\n" +
		"Binary files a/bin and b/bin differ\n"
	blocks, err := splitBlocks([]byte(patch))
	if err != nil {
		t.Fatalf("splitBlocks: %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	h := blocks[0].hunks[0]
	if blocks[0].binary || len(h.Removed) != 1 || h.Removed[0] != (Line{2, "old"}) || len(h.Added) != 1 || h.Added[0] != (Line{2, "new"}) {
		t.Errorf("text block = %+v", blocks[0])
	}
	if !blocks[1].binary || !blocks[2].binary {
		t.Errorf("binary flags = %v, %v; want both true", blocks[1].binary, blocks[2].binary)
	}
}

func TestAttachPatchesErrors(t *testing.T) {
	one := []File{{Path: "x", Status: StatusModified}}
	tests := []struct {
		name  string
		files []File
		patch string
		want  string
	}{
		{"bad patch", one, "junk\n", "expected diff header"},
		{"no patch for file", one, "", `no patch for "x"`},
		{"header names another file", one, "diff --git a/y b/y\n", "patch header"},
		{"rename header must name both paths", []File{{Path: "b", OldPath: "a", Status: StatusRenamed}}, "diff --git a/b b/b\n", "patch header"},
		{"type change needs two blocks", []File{{Path: "x", Status: StatusTypeChanged}}, "diff --git a/x b/x\n", `no patch for "x"`},
		{"left over blocks", one, "diff --git a/x b/x\ndiff --git a/z b/z\n", "1 patch blocks left over"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := attachPatches(slices.Clone(tt.files), []byte(tt.patch))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("attachPatches error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestFromGitErrors(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base")
	ctx := context.Background()

	if _, err := FromGit(ctx, r.dir, base, base, Options{Git: "/nonexistent/git"}); err == nil || !strings.Contains(err.Error(), "compute empty tree") {
		t.Errorf("missing git binary: error = %v", err)
	}
	if _, err := FromGit(ctx, t.TempDir(), base, base, Options{}); err == nil || !strings.Contains(err.Error(), "revision") {
		t.Errorf("not a repository: error = %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := FromGit(canceled, r.dir, base, base, Options{}); err == nil {
		t.Error("canceled context: want an error")
	}
}

// fakeGit returns a git wrapper that runs action, a shell snippet, for any
// call whose arguments contain match, and the real git ($real) otherwise.
func fakeGit(t *testing.T, match, action string) string {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "git")
	body := "#!/bin/sh\nreal=" + real + "\ncase \" $* \" in *\" " + match + " \"*) " + action + ";; esac\nexec \"$real\" \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

const injectFailure = "echo injected failure >&2; exit 1"

func TestFromGitStepFailures(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "one\n")
	base := r.commit("base")
	r.write("a.txt", "two\n")
	head := r.commit("head")

	tests := []struct{ match, want string }{
		{"cat-file -t", "revision"},
		{"--raw", "list changes"},
		{"-p", "diff"},
		{"--batch", "read blobs"},
	}
	for _, tt := range tests {
		t.Run(tt.match, func(t *testing.T) {
			_, err := FromGit(context.Background(), r.dir, base, head, Options{Git: fakeGit(t, tt.match, injectFailure)})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestVerifyCommitRejectsOtherObjects(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "one\n")
	r.commit("base")
	tree := r.git("rev-parse", "HEAD^{tree}")
	_, err := FromGit(context.Background(), r.dir, tree, tree, Options{})
	if err == nil || !strings.Contains(err.Error(), "is a tree, not a commit") {
		t.Errorf("error = %v, want a not-a-commit error", err)
	}
}

func TestReadBlobsReportsGitExitStatus(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "one\n")
	base := r.commit("base")
	r.write("a.txt", "two\n")
	head := r.commit("head")

	// The batch output is complete, but git then exits non-zero.
	git := fakeGit(t, "--batch", `"$real" "$@"; echo injected failure >&2; exit 1`)
	_, err := FromGit(context.Background(), r.dir, base, head, Options{Git: git})
	if err == nil || !strings.Contains(err.Error(), "git cat-file") || !strings.Contains(err.Error(), "injected failure") {
		t.Errorf("error = %v, want the cat-file exit status with its stderr", err)
	}
}

func TestIsBinary(t *testing.T) {
	for in, want := range map[string]bool{"\x00ELF": true, "text\x00": true, "plain text": false, "": false} {
		if got := isBinary([]byte(in)); got != want {
			t.Errorf("isBinary(%q) = %v, want %v", in, got, want)
		}
	}
}
