package changes

import (
	"reflect"
	"testing"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/fixture"
)

func TestBuildConvertsEveryField(t *testing.T) {
	c := fixture.Change{Files: []fixture.File{
		{
			Path:    "bin/run",
			OldPath: "run.sh",
			Status:  fixture.StatusRenamed,
			OldMode: changeset.ModeRegular,
			NewMode: changeset.ModeExecutable,
			Binary:  true,
			LFS:     true,
			Size:    42,
			Hunks: []fixture.Hunk{
				{Added: []string{"a", "b"}, Removed: []string{"c"}},
				{Added: []string{"d"}},
			},
		},
		{Path: "README.md"},
	}}

	want := changeset.ChangeSet{Files: []changeset.File{
		{
			Path:    "bin/run",
			OldPath: "run.sh",
			Status:  changeset.StatusRenamed,
			OldMode: changeset.ModeRegular,
			NewMode: changeset.ModeExecutable,
			NewSize: 42,
			Binary:  true,
			LFS:     true,
			Hunks: []changeset.Hunk{
				{
					Added:   []changeset.Line{{Line: 1, Text: "a"}, {Line: 2, Text: "b"}},
					Removed: []changeset.Line{{Line: 1, Text: "c"}},
				},
				{Added: []changeset.Line{{Line: 1, Text: "d"}}},
			},
		},
		{Path: "README.md", Status: changeset.StatusModified},
	}}
	if got := Build(c); !reflect.DeepEqual(got, want) {
		t.Errorf("Build =\n%+v\nwant\n%+v", got, want)
	}
}

func TestBuildAddsPlaceholderLines(t *testing.T) {
	tests := []struct {
		name           string
		added, removed int
	}{
		{"added only", 2, 0},
		{"removed only", 0, 1},
		{"both", 3, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Build(fixture.Change{Files: []fixture.File{{
				Path:  "a.go",
				Hunks: []fixture.Hunk{{Added: []string{"x"}}},
				Added: tt.added, Removed: tt.removed,
			}}}).Files[0]
			if len(f.Hunks) != 2 {
				t.Fatalf("got %d hunks, want the fixture's hunk plus one placeholder hunk", len(f.Hunks))
			}
			if f.AddedLines() != 1+tt.added || f.RemovedLines() != tt.removed {
				t.Errorf("added %d, removed %d; want %d, %d", f.AddedLines(), f.RemovedLines(), 1+tt.added, tt.removed)
			}
			if tt.added > 0 {
				last := f.Hunks[1].Added[tt.added-1]
				if want := (changeset.Line{Line: tt.added, Text: "line " + itoa(tt.added)}); last != want {
					t.Errorf("last placeholder = %+v, want %+v", last, want)
				}
			}
		})
	}
}

func TestBuildAddsNoPlaceholderHunkWithoutCounts(t *testing.T) {
	f := Build(fixture.Change{Files: []fixture.File{{Path: "a.go"}}}).Files[0]
	if f.Hunks != nil {
		t.Errorf("hunks = %+v, want none", f.Hunks)
	}
}

func itoa(n int) string { return string(rune('0' + n)) }
