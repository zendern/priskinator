// Package changeset builds the normalized change set for a pull request from git, reading the head only as data (ADR 0005).
package changeset

// Status is how a file changed between base and head. The names match the
// golden-fixture format (internal/fixture).
type Status string

const (
	StatusAdded       Status = "added"
	StatusModified    Status = "modified"
	StatusDeleted     Status = "deleted"
	StatusRenamed     Status = "renamed"
	StatusCopied      Status = "copied"
	StatusTypeChanged Status = "type_changed"
)

// Git file modes as they appear in trees.
const (
	ModeRegular    = "100644"
	ModeExecutable = "100755"
	ModeSymlink    = "120000"
	ModeSubmodule  = "160000"
)

// Kind is the type of tree entry a file is.
type Kind string

const (
	KindRegular    Kind = "regular"
	KindExecutable Kind = "executable"
	KindSymlink    Kind = "symlink"
	KindSubmodule  Kind = "submodule"
	KindUnknown    Kind = "unknown"
)

// ChangeSet is every file that differs between two commits.
type ChangeSet struct {
	Base  string
	Head  string
	Files []File
}

// File is one changed path. Old* fields describe the base side and New* fields
// the head side; the side that does not exist has empty mode and OID and zero size.
type File struct {
	// Path is the head path, or the base path for a deletion.
	Path string
	// OldPath is set for renames and copies only.
	OldPath string
	Status  Status

	OldMode string
	NewMode string
	OldOID  string
	NewOID  string
	OldSize int64
	NewSize int64

	// Similarity is git's rename or copy score, 0-100.
	Similarity int
	// Binary is true when either side's content is binary.
	Binary bool
	// LFS is true when either side is a Git LFS pointer.
	LFS bool
	// SymlinkTarget is the head-side target when the head side is a symlink.
	SymlinkTarget string

	// Hunks hold added and removed lines. Binary files and submodules have none.
	Hunks []Hunk
}

// Hunk is one contiguous block of changes.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	// Removed lines are numbered in the base file, added lines in the head file.
	Removed []Line
	Added   []Line
}

// Line is a line of text and its 1-based line number.
type Line struct {
	Line int
	Text string
}

// Kind returns the head-side kind, or the base-side kind for a deletion.
func (f File) Kind() Kind {
	mode := f.NewMode
	if mode == "" {
		mode = f.OldMode
	}
	return kindOf(mode)
}

func kindOf(mode string) Kind {
	switch mode {
	case ModeRegular:
		return KindRegular
	case ModeExecutable:
		return KindExecutable
	case ModeSymlink:
		return KindSymlink
	case ModeSubmodule:
		return KindSubmodule
	default:
		return KindUnknown
	}
}

// ModeChanged reports whether both sides exist and their modes differ.
func (f File) ModeChanged() bool {
	return f.OldMode != "" && f.NewMode != "" && f.OldMode != f.NewMode
}

// Paths returns the head path and, for renames and copies, the base path.
// Path rules match against both so a move out of a watched path still fires.
func (f File) Paths() []string {
	if f.OldPath != "" && f.OldPath != f.Path {
		return []string{f.Path, f.OldPath}
	}
	return []string{f.Path}
}

// AddedLines counts added lines across hunks.
func (f File) AddedLines() int {
	n := 0
	for _, h := range f.Hunks {
		n += len(h.Added)
	}
	return n
}

// RemovedLines counts removed lines across hunks.
func (f File) RemovedLines() int {
	n := 0
	for _, h := range f.Hunks {
		n += len(h.Removed)
	}
	return n
}
