package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func addr[T any](v T) *T { return &v }

func TestSchemaJSONIsACopy(t *testing.T) {
	got := SchemaJSON()
	if !json.Valid(got) {
		t.Fatal("schema is not valid JSON")
	}
	got[0] = 'x'
	if !bytes.Equal(SchemaJSON(), schemaJSON) {
		t.Error("changing the returned schema changed the embedded one")
	}
}

func TestTriggerIsEnabled(t *testing.T) {
	cases := map[string]struct {
		enabled *bool
		want    bool
	}{
		"unset":    {nil, true},
		"enabled":  {addr(true), true},
		"disabled": {addr(false), false},
	}
	for name, tc := range cases {
		if got := (&Trigger{Enabled: tc.enabled}).IsEnabled(); got != tc.want {
			t.Errorf("%s: IsEnabled() = %v, want %v", name, got, tc.want)
		}
	}
}

func TestBandsPoints(t *testing.T) {
	bands := Bands{{UpTo: addr(10), Points: 1}, {UpTo: addr(20), Points: 5}, {Points: 9}}
	for value, want := range map[int]int{0: 1, 10: 1, 11: 5, 20: 5, 21: 9, 1000: 9} {
		if got := bands.Points(value); got != want {
			t.Errorf("Points(%d) = %d, want %d", value, got, want)
		}
	}
	if got := (Bands{{UpTo: addr(10), Points: 1}}).Points(11); got != 0 {
		t.Errorf("value past every band: Points = %d, want 0", got)
	}
	if got := (Bands{}).Points(5); got != 0 {
		t.Errorf("no bands: Points = %d, want 0", got)
	}
}

func TestMaxRiskKeepsFirstOfEqualRank(t *testing.T) {
	if got := MaxRisk(); got != RiskLow {
		t.Errorf("MaxRisk() = %q, want low", got)
	}
	// Unknown values rank with high; the first one seen wins.
	if got := MaxRisk(Risk("severe"), RiskHigh); got != Risk("severe") {
		t.Errorf("MaxRisk(severe, high) = %q, want severe", got)
	}
}

func TestParseAcceptsInputAtTheSizeLimit(t *testing.T) {
	head := "version: 1\n# "
	data := head + strings.Repeat("x", MaxPolicyBytes-len(head)-1) + "\n"
	if len(data) != MaxPolicyBytes {
		t.Fatalf("test input is %d bytes, want %d", len(data), MaxPolicyBytes)
	}
	if _, err := Parse([]byte(data)); err != nil {
		t.Errorf("Parse at the limit: %v", err)
	}
}

func TestParseNamesTopLevelType(t *testing.T) {
	for doc, want := range map[string]string{"hello\n": "string", "true\n": "boolean", "3\n": "number"} {
		_, err := Parse([]byte(doc))
		if err == nil || !strings.Contains(err.Error(), "expected object, got "+want) {
			t.Errorf("Parse(%q) = %v, want expected object, got %s", doc, err, want)
		}
	}
}

func TestParseSortsProblemsByPath(t *testing.T) {
	cases := map[string]string{
		// Schema problems.
		"schema": "version: 1\nscoring:\n  floors:\n    - risk: bogus\n      paths: [a]\nai:\n  provider: skynet\nlimits:\n  max_file_bytes: -1\n",
		// Problems the schema cannot express.
		"checks": "version: 1\nscoring:\n  thresholds: {medium: 5, high: 1}\n  exclude_paths: [\"/abs\"]\ntriggers:\n  repo.x:\n    paths: [\"/abs\"]\n",
	}
	for name, doc := range cases {
		_, err := Parse([]byte(doc))
		var perr *Error
		if !errors.As(err, &perr) || len(perr.Problems) < 2 {
			t.Fatalf("%s: want several problems, got %v", name, err)
		}
		if !slices.IsSortedFunc(perr.Problems, func(a, b Problem) int { return strings.Compare(a.Path, b.Path) }) {
			t.Errorf("%s: problems not sorted by path: %+v", name, perr.Problems)
		}
	}
}

func TestParseBandAndThresholdEdges(t *testing.T) {
	cases := map[string]struct {
		yaml string
		want string // substring of the error, or "" for valid
	}{
		"equal thresholds":    {"version: 1\nscoring:\n  thresholds: {medium: 20, high: 20}\n", "medium must be below high"},
		"equal up_to":         {"version: 1\nscoring:\n  factors:\n    size:\n      - {up_to: 50, points: 0}\n      - {up_to: 50, points: 5}\n      - {points: 10}\n", "ascending"},
		"equal points":        {"version: 1\nscoring:\n  factors:\n    size:\n      - {up_to: 50, points: 5}\n      - {points: 5}\n", ""},
		"single open band":    {"version: 1\nscoring:\n  factors:\n    size:\n      - {points: 5}\n", ""},
		"ascending bands":     {"version: 1\nscoring:\n  factors:\n    size:\n      - {up_to: 10, points: 0}\n      - {up_to: 20, points: 5}\n      - {points: 10}\n", ""},
		"thresholds in order": {"version: 1\nscoring:\n  thresholds: {medium: 19, high: 20}\n", ""},
	}
	for name, tc := range cases {
		_, err := Parse([]byte(tc.yaml))
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
}

func TestObjectOf(t *testing.T) {
	fallback := []string{"fallback"}
	cases := map[string][]string{
		"policy.json#/properties/triggers/propertyNames":                   {"triggers"},
		"policy.json#/properties/scoring/properties/factors/propertyNames": {"scoring", "factors"},
		"policy.json#/properties/triggers/additionalProperties":            fallback,
		"policy.json#/properties/propertyNames":                            fallback,
		"policy.json#/properties/a/items/propertyNames":                    fallback,
		"policy.json#/propertyNames":                                       nil,
		"policy.json":                                                      fallback,
	}
	for url, want := range cases {
		if got := objectOf(url, fallback); !slices.Equal(got, want) {
			t.Errorf("objectOf(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestValueAt(t *testing.T) {
	doc := map[string]any{
		"a": []any{"x", json.Number("2"), map[string]any{"b": true}},
		"s": "str",
	}
	cases := []struct {
		loc  []string
		want string
		ok   bool
	}{
		{[]string{"s"}, `"str"`, true},
		{[]string{"a", "0"}, `"x"`, true},
		{[]string{"a", "1"}, `2`, true},
		{[]string{"a", "2", "b"}, `true`, true},
		{[]string{"a", "2"}, "", false},
		{[]string{"a"}, "", false},
		{[]string{"a", "3"}, "", false},
		{[]string{"a", "-1"}, "", false},
		{[]string{"a", "x"}, "", false},
		{[]string{"s", "deeper"}, "", false},
	}
	for _, tc := range cases {
		got, ok := valueAt(doc, tc.loc)
		if got != tc.want || ok != tc.ok {
			t.Errorf("valueAt(%q) = %q, %v; want %q, %v", tc.loc, got, ok, tc.want, tc.ok)
		}
	}
}
