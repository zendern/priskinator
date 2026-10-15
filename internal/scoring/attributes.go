package scoring

import "strings"

// Generated holds the paths a base-branch .gitattributes marks as generated or
// vendored (linguist-generated, linguist-vendored), which scoring leaves out
// (ADR 0007).
type Generated struct {
	include []string
	exclude []string
}

// ParseAttributes reads the linguist-generated and linguist-vendored
// attributes from a .gitattributes file. A path is generated when it matches a
// line that sets either attribute and no line that unsets one. That is
// stricter than git's last-match-wins, so an unset line anywhere keeps a path
// scored. Macro definitions, quoted patterns, and directory patterns (which
// never match files in .gitattributes) are skipped.
func ParseAttributes(data []byte) Generated {
	var g Generated
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], "[attr]") ||
			strings.HasPrefix(fields[0], `"`) || strings.HasSuffix(fields[0], "/") {
			continue
		}
		glob := toGlob(fields[0])
		for _, attr := range fields[1:] {
			set, ok := linguist(attr)
			if !ok {
				continue
			}
			if set {
				g.include = append(g.include, glob)
			} else {
				g.exclude = append(g.exclude, glob)
			}
		}
	}
	return g
}

// linguist reports whether attr sets or unsets a linguist generated or
// vendored attribute.
func linguist(attr string) (set, ok bool) {
	name, value, hasValue := strings.Cut(attr, "=")
	set = !hasValue || value == "true"
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "!") {
		name, set = name[1:], false
	}
	return set, name == "linguist-generated" || name == "linguist-vendored"
}

// toGlob turns a gitattributes pattern into a doublestar glob: a leading slash
// anchors it at the root, and a pattern with no slash matches at any depth.
func toGlob(pattern string) string {
	if strings.HasPrefix(pattern, "/") {
		return pattern[1:]
	}
	if !strings.Contains(pattern, "/") {
		return "**/" + pattern
	}
	return pattern
}

// Matches reports whether p is generated or vendored.
func (g Generated) Matches(p string) bool {
	return matchAny(g.include, p, false) && !matchAny(g.exclude, p, false)
}
