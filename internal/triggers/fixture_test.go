package triggers

import (
	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/fixture"
)

// changeSet builds the change set a fixture describes. Hunk lines are numbered
// from 1 within each hunk, and an unset status means modified.
func changeSet(c fixture.Change) changeset.ChangeSet {
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
		files = append(files, changeset.File{
			Path:    f.Path,
			OldPath: f.OldPath,
			Status:  status,
			OldMode: f.OldMode,
			NewMode: f.NewMode,
			NewSize: f.Size,
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
