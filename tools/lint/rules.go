//go:build ruleguard

// Package gorules holds gocritic ruleguard rules (see .golangci.yml).
package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// taglessSwitch bans `switch { case cond: ... }`. gremlins cannot mutation
// test case conditions: Go's coverage profile has no statement on a case
// line, so every mutant there is reported NOT COVERED and never run. Use if
// statements (guard clauses read best). Tagged and type switches are fine:
// their cases are constants and types, which gremlins has nothing to mutate.
// Tests are not mutated, so they may use any switch.
func taglessSwitch(m dsl.Matcher) {
	m.Match(`switch { $*_ }`, `switch $_; { $*_ }`).
		Where(!m.File().Name.Matches(`_test\.go$`)).
		Report(`use if statements instead of a tagless switch: gremlins cannot mutate case conditions`)
}
