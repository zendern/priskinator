package changeset

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// attachPatches splits `diff-tree -p` output into per-file blocks and attaches
// their hunks to files, which come from the raw output of the same diff and are
// in the same order. Each block's header must name the expected paths, so a
// mismatch fails instead of attributing lines to the wrong file.
//
// Git prints a type change as two blocks: a deletion and then a creation.
func attachPatches(files []File, patch []byte) error {
	blocks, err := splitBlocks(patch)
	if err != nil {
		return err
	}
	next := 0
	take := func(f File) (block, error) {
		if next >= len(blocks) {
			return block{}, fmt.Errorf("no patch for %q", f.Path)
		}
		b := blocks[next]
		next++
		oldPath := f.Path
		if f.OldPath != "" {
			oldPath = f.OldPath
		}
		want := "diff --git " + quotePath("a/"+oldPath) + " " + quotePath("b/"+f.Path)
		if b.header != want {
			return block{}, fmt.Errorf("patch header %q, want %q", b.header, want)
		}
		return b, nil
	}
	for i := range files {
		f := &files[i]
		n := 1
		if f.Status == StatusTypeChanged {
			n = 2
		}
		for range n {
			b, err := take(*f)
			if err != nil {
				return err
			}
			if b.binary {
				f.Binary = true
			}
			f.Hunks = append(f.Hunks, b.hunks...)
		}
	}
	if next != len(blocks) {
		return fmt.Errorf("%d patch blocks left over", len(blocks)-next)
	}
	return nil
}

type block struct {
	header string
	binary bool
	hunks  []Hunk
}

func splitBlocks(patch []byte) ([]block, error) {
	lines := strings.Split(string(patch), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	var blocks []block
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "diff --git ") {
			return nil, fmt.Errorf("line %d: expected diff header, got %q", i+1, lines[i])
		}
		b := block{header: lines[i]}
		i++
		// Extended header lines up to the first hunk or the next file.
		for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") && !strings.HasPrefix(lines[i], "diff --git ") {
			if strings.HasPrefix(lines[i], "Binary files ") || lines[i] == "GIT binary patch" {
				b.binary = true
			}
			i++
		}
		// Hunks. Their headers give exact line counts, so content lines are
		// consumed by count and never mistaken for headers.
		for i < len(lines) && strings.HasPrefix(lines[i], "@@ ") {
			h, err := parseHunkHeader(lines[i])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			i++
			oldLine, newLine := h.OldStart, h.NewStart
			for oldLine < h.OldStart+h.OldLines || newLine < h.NewStart+h.NewLines {
				if i >= len(lines) {
					return nil, fmt.Errorf("hunk %q truncated", b.header)
				}
				l := lines[i]
				i++
				if strings.HasPrefix(l, `\`) {
					continue // "\ No newline at end of file"
				}
				if strings.HasPrefix(l, "-") {
					h.Removed = append(h.Removed, Line{Line: oldLine, Text: l[1:]})
					oldLine++
					continue
				}
				if strings.HasPrefix(l, "+") {
					h.Added = append(h.Added, Line{Line: newLine, Text: l[1:]})
					newLine++
					continue
				}
				if !strings.HasPrefix(l, " ") {
					return nil, fmt.Errorf("unexpected hunk line %q in %q", l, b.header)
				}
				oldLine++
				newLine++
			}
			for i < len(lines) && strings.HasPrefix(lines[i], `\`) {
				i++
			}
			b.hunks = append(b.hunks, h)
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// parseHunkHeader reads "@@ -old[,count] +new[,count] @@".
func parseHunkHeader(s string) (Hunk, error) {
	rest, ok := strings.CutPrefix(s, "@@ -")
	if !ok {
		return Hunk{}, fmt.Errorf("bad hunk header %q", s)
	}
	end := strings.Index(rest, " @@")
	if end < 0 {
		return Hunk{}, fmt.Errorf("bad hunk header %q", s)
	}
	oldR, newR, ok := strings.Cut(rest[:end], " +")
	if !ok {
		return Hunk{}, fmt.Errorf("bad hunk header %q", s)
	}
	var h Hunk
	var err error
	if h.OldStart, h.OldLines, err = parseRange(oldR); err != nil {
		return Hunk{}, fmt.Errorf("bad hunk header %q: %w", s, err)
	}
	if h.NewStart, h.NewLines, err = parseRange(newR); err != nil {
		return Hunk{}, fmt.Errorf("bad hunk header %q: %w", s, err)
	}
	// An empty side is reported at the line before the change.
	if h.OldLines == 0 {
		h.OldStart++
	}
	if h.NewLines == 0 {
		h.NewStart++
	}
	return h, nil
}

func parseRange(s string) (start, count int, err error) {
	startS, countS, hasCount := strings.Cut(s, ",")
	if start, err = strconv.Atoi(startS); err != nil {
		return 0, 0, err
	}
	count = 1
	if hasCount {
		if count, err = strconv.Atoi(countS); err != nil {
			return 0, 0, err
		}
	}
	if start < 0 || count < 0 {
		return 0, 0, fmt.Errorf("negative range %q", s)
	}
	return start, count, nil
}

// quotePath quotes a path the way git does in patch headers with
// core.quotePath=false: C-style quoting only when the path contains a double
// quote, a backslash, or a control character.
func quotePath(p string) string {
	needs := false
	for i := 0; i < len(p); i++ {
		if c := p[i]; c == '"' || c == '\\' || c < 0x20 || c == 0x7f {
			needs = true
			break
		}
	}
	if !needs {
		return p
	}
	var b bytes.Buffer
	b.WriteByte('"')
	for i := 0; i < len(p); i++ {
		switch c := p[i]; c {
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\v':
			b.WriteString(`\v`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			if c < 0x20 || c == 0x7f {
				fmt.Fprintf(&b, `\%03o`, c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
