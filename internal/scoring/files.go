package scoring

import (
	"path"
	"slices"
	"strings"
	"unicode"

	enry "github.com/go-enry/go-enry/v2"

	"github.com/zendern/priskinator/internal/changeset"
)

// isSource reports whether f is code, using GitHub linguist's language data
// (go-enry). Its added lines are a content sample, so an ambiguous extension
// such as .rs is told apart from XML. A file of unknown type counts as source,
// so an unrecognized extension errs toward the test signal firing. Prose, data,
// and markup do not count.
func isSource(f changeset.File) bool {
	var sample strings.Builder
	for _, h := range f.Hunks {
		for _, l := range h.Added {
			sample.WriteString(l.Text)
			sample.WriteByte('\n')
		}
	}
	t := enry.GetLanguageType(enry.GetLanguage(path.Base(f.Path), []byte(sample.String())))
	return t == enry.Programming || t == enry.Unknown
}

// testWords mark a test file when they appear as a whole word anywhere in its
// path, in any case. Matching words rather than substrings keeps names such as
// latest/, attestation.go, and inspect.py from passing as tests.
var testWords = []string{"test", "tests", "spec", "specs"}

func isTest(p string) bool {
	if path.Base(p) == "conftest.py" {
		return true
	}
	return slices.ContainsFunc(words(p), func(w string) bool { return slices.Contains(testWords, w) })
}

// words splits a path into lowercase words at every non-letter, non-digit
// character and at camelCase boundaries: "src/FooTest.java" gives src, foo,
// test, java, and "HTTPSpec" gives http, spec.
func words(p string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(p)
	for i, r := range rs {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if unicode.IsUpper(r) && i > 0 && startsWord(rs, i) {
			flush()
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

// startsWord reports whether the uppercase rune at i begins a new camelCase
// word: after a lowercase letter or digit ("fooTest"), or as the last capital
// of an acronym before a lowercase letter ("HTTPSpec").
func startsWord(rs []rune, i int) bool {
	prev := rs[i-1]
	if unicode.IsLower(prev) || unicode.IsDigit(prev) {
		return true
	}
	return unicode.IsUpper(prev) && i+1 < len(rs) && unicode.IsLower(rs[i+1])
}
