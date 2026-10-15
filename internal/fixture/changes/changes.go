// Package changes builds a changeset.ChangeSet from a golden fixture, for the
// engines that run fixtures. It is separate from package fixture because
// changeset's own tests import fixture.
package changes

import (
	"strconv"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/fixture"
)

// Build returns the change set a fixture describes. Hunk lines are numbered
// from 1 within each hunk, an unset status means modified, and a file's added
// and removed counts become one extra hunk of placeholder lines.
func Build(c fixture.Change) changeset.ChangeSet {
	files := make([]changeset.File, 0, len(c.Files))
	for _, f := range c.Files {
		status := changeset.Status(f.Status)
		if status == "" {
			status = changeset.StatusModified
		}
		var hunks []changeset.Hunk
		for _, h := range f.Hunks {
			hunks = append(hunks, changeset.Hunk{Added: lines(h.Added), Removed: lines(h.Removed)})
		}
		if f.Added > 0 || f.Removed > 0 {
			hunks = append(hunks, changeset.Hunk{Added: placeholders(f.Added), Removed: placeholders(f.Removed)})
		}
		files = append(files, changeset.File{
			Path:    f.Path,
			OldPath: f.OldPath,
			Status:  status,
			OldMode: f.OldMode,
			NewMode: f.NewMode,
			Size:    f.Size,
			Binary:  f.Binary,
			LFS:     f.LFS,
			Hunks:   hunks,
		})
	}
	return changeset.ChangeSet{Files: files}
}

func lines(texts []string) []changeset.Line {
	var out []changeset.Line
	for i, t := range texts {
		out = append(out, changeset.Line{Line: i + 1, Text: t})
	}
	return out
}

func placeholders(n int) []changeset.Line {
	texts := make([]string, n)
	for i := range texts {
		texts[i] = "line " + strconv.Itoa(i+1)
	}
	return lines(texts)
}
