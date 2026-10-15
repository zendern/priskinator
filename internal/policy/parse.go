package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// ErrUnsupportedVersion is returned for a policy whose version this build does not read.
var ErrUnsupportedVersion = errors.New("unsupported policy version")

// Error lists every problem found in a policy.
type Error struct {
	Problems []Problem
}

// Problem is one validation failure. Path is a JSON pointer into the policy.
type Problem struct {
	Path    string
	Message string
}

func (e *Error) Error() string {
	msgs := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		loc := p.Path
		if loc == "" {
			loc = "/"
		}
		msgs[i] = loc + ": " + p.Message
	}
	return "invalid policy: " + strings.Join(msgs, "; ")
}

var schema = compileSchema()

func compileSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		panic("policy schema: " + err.Error())
	}
	c := jsonschema.NewCompiler()
	const url = "https://github.com/zendern/priskinator/schemas/policy/v1.json"
	if err := c.AddResource(url, doc); err != nil {
		panic("policy schema: " + err.Error())
	}
	s, err := c.Compile(url)
	if err != nil {
		panic("policy schema: " + err.Error())
	}
	return s
}

// Parse reads one policy layer. It checks, in order: a single YAML document
// with no duplicate keys, a supported version, the JSON Schema (unknown keys
// are errors), and rules the schema cannot express (globs, band order,
// thresholds, new rules having paths). Every problem found is reported.
func Parse(data []byte) (*Policy, error) {
	if len(data) > MaxPolicyBytes {
		return nil, fmt.Errorf("policy is larger than %d bytes", MaxPolicyBytes)
	}
	doc, err := decodeYAML(data)
	if err != nil {
		return nil, err
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return nil, &Error{Problems: []Problem{{Message: fmt.Sprintf("expected object, got %s", jsonType(doc))}}}
	}
	if v, ok := obj["version"].(json.Number); ok {
		if n, err := v.Int64(); err == nil && n != Version {
			return nil, fmt.Errorf("%w %d (this build reads version %d)", ErrUnsupportedVersion, n, Version)
		}
	}

	if err := schema.Validate(doc); err != nil {
		var verr *jsonschema.ValidationError
		if !errors.As(err, &verr) {
			return nil, err
		}
		return nil, &Error{Problems: schemaProblems(doc, verr)}
	}

	var p Policy
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode policy: %w", err)
	}
	if problems := check(&p); len(problems) > 0 {
		return nil, &Error{Problems: problems}
	}
	return &p, nil
}

// decodeYAML decodes exactly one YAML document into JSON-compatible values.
func decodeYAML(data []byte) (any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := dec.Decode(&node); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("empty policy")
		}
		return nil, fmt.Errorf("parse policy: %w", err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("policy must be a single YAML document")
	}
	var raw any
	if err := node.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse policy: %w", err)
	}
	if raw == nil {
		return nil, errors.New("empty policy")
	}
	// Round-trip through JSON so the schema sees exactly JSON types.
	conv, err := toJSONValue(raw, "")
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(conv)
	if err != nil {
		return nil, fmt.Errorf("parse policy: %w", err)
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(buf))
}

func toJSONValue(v any, path string) (any, error) {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			c, err := toJSONValue(e, path+"/"+k)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	case map[any]any:
		for k := range v {
			if _, ok := k.(string); !ok {
				return nil, &Error{Problems: []Problem{{Path: path, Message: fmt.Sprintf("key %v is not a string", k)}}}
			}
		}
		out := make(map[string]any, len(v))
		for k, e := range v {
			c, err := toJSONValue(e, path+"/"+k.(string))
			if err != nil {
				return nil, err
			}
			out[k.(string)] = c
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			c, err := toJSONValue(e, path+"/"+strconv.Itoa(i))
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case string, bool, int, int64, uint64, float64, nil:
		return v, nil
	default:
		return nil, &Error{Problems: []Problem{{Path: path, Message: fmt.Sprintf("unsupported value of type %T", v)}}}
	}
}

func jsonType(v any) string {
	switch v.(type) {
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// schemaProblems flattens the schema error tree into leaf problems, each
// located by a JSON pointer and naming the offending value where useful.
func schemaProblems(doc any, verr *jsonschema.ValidationError) []Problem {
	printer := message.NewPrinter(language.English)
	var out []Problem
	var walk func(e *jsonschema.ValidationError, parent []string)
	walk = func(e *jsonschema.ValidationError, parent []string) {
		loc := e.InstanceLocation
		switch e.ErrorKind.(type) {
		case *kind.PropertyNames:
			// The library locates key-name errors oddly; the key belongs to the
			// object that encloses this error.
			msg := e.ErrorKind.LocalizedString(printer)
			for _, c := range e.Causes {
				msg += ": " + c.ErrorKind.LocalizedString(printer)
			}
			out = append(out, Problem{Path: pointer(objectOf(e.SchemaURL, parent)), Message: msg})
			return
		case *kind.Enum, *kind.Const, *kind.Type:
			msg := e.ErrorKind.LocalizedString(printer)
			if v, ok := valueAt(doc, loc); ok {
				msg = fmt.Sprintf("%s: %s", v, msg)
			}
			out = append(out, Problem{Path: pointer(loc), Message: msg})
			return
		}
		if len(e.Causes) == 0 {
			out = append(out, Problem{Path: pointer(loc), Message: e.ErrorKind.LocalizedString(printer)})
			return
		}
		for _, c := range e.Causes {
			walk(c, loc)
		}
	}
	walk(verr, nil)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// objectOf finds the instance object a propertyNames keyword applies to from
// its schema location (".../properties/triggers/propertyNames" is /triggers).
// It falls back to the enclosing error's location.
func objectOf(schemaURL string, fallback []string) []string {
	_, frag, ok := strings.Cut(schemaURL, "#")
	if !ok {
		return fallback
	}
	segs := strings.Split(strings.Trim(frag, "/"), "/")
	if len(segs) == 0 || segs[len(segs)-1] != "propertyNames" {
		return fallback
	}
	var loc []string
	for i := 0; i < len(segs)-1; i += 2 {
		if segs[i] != "properties" || i+1 >= len(segs)-1 {
			return fallback
		}
		loc = append(loc, segs[i+1])
	}
	return loc
}

func pointer(loc []string) string {
	if len(loc) == 0 {
		return ""
	}
	return "/" + strings.Join(loc, "/")
}

// valueAt renders the scalar at loc, quoted like JSON.
func valueAt(doc any, loc []string) (string, bool) {
	v := doc
	for _, k := range loc {
		switch c := v.(type) {
		case map[string]any:
			v = c[k]
		case []any:
			i, err := strconv.Atoi(k)
			if err != nil || i < 0 || i >= len(c) {
				return "", false
			}
			v = c[i]
		default:
			return "", false
		}
	}
	switch v.(type) {
	case map[string]any, []any:
		return "", false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// check enforces what the schema cannot express.
func check(p *Policy) []Problem {
	var out []Problem
	add := func(path, format string, args ...any) {
		out = append(out, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
	}
	globs := func(path string, patterns []string) {
		for i, g := range patterns {
			at := path + "/" + strconv.Itoa(i)
			if strings.Contains(g, `\`) {
				add(at, "glob %q must use forward slashes", g)
				continue
			}
			if strings.HasPrefix(g, "/") || strings.HasPrefix(g, "./") {
				add(at, "glob %q must be relative to the repository root, without a leading / or ./", g)
				continue
			}
			if !doublestar.ValidatePattern(g) {
				add(at, "invalid glob %q", g)
			}
		}
	}

	ids := make([]string, 0, len(p.Triggers))
	for id := range p.Triggers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t := p.Triggers[id]
		at := "/triggers/" + id
		if t == nil {
			add(at, "rule %s is empty", id)
			continue
		}
		globs(at+"/paths", t.Paths)
		globs(at+"/add_paths", t.AddPaths)
		if isCustomRule(id) && len(t.Paths) == 0 {
			add(at, "new rule %s needs paths; content rules are built in", id)
		}
		if isCustomRule(id) && len(t.AddPaths) > 0 {
			add(at+"/add_paths", "add_paths extends a baseline rule; use paths for new rule %s", id)
		}
	}

	if th := p.Scoring.Thresholds; th != nil && th.Medium >= th.High {
		add("/scoring/thresholds", "medium must be below high (got %d and %d)", th.Medium, th.High)
	}
	f := p.Scoring.Factors
	for name, b := range map[string]Bands{
		"size": f.Size, "files": f.Files, "diffusion": f.Diffusion,
		"complexity_delta": f.ComplexityDelta, "test_signal": f.TestSignal,
	} {
		out = append(out, checkBands("/scoring/factors/"+name, b)...)
	}
	for i, fl := range p.Scoring.Floors {
		globs("/scoring/floors/"+strconv.Itoa(i)+"/paths", fl.Paths)
	}
	globs("/scoring/exclude_paths", p.Scoring.ExcludePaths)

	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func checkBands(path string, bands Bands) []Problem {
	var out []Problem
	add := func(i int, format string, args ...any) {
		out = append(out, Problem{Path: path + "/" + strconv.Itoa(i), Message: fmt.Sprintf(format, args...)})
	}
	for i, b := range bands {
		last := i == len(bands)-1
		if b.UpTo == nil && !last {
			add(i, "only the last band may omit up_to")
		}
		if b.UpTo != nil && last {
			add(i, "the last band must omit up_to so every value has points")
		}
		if i == 0 {
			continue
		}
		prev := bands[i-1]
		if b.UpTo != nil && prev.UpTo != nil && *b.UpTo <= *prev.UpTo {
			add(i, "up_to values must be strictly ascending (%d after %d)", *b.UpTo, *prev.UpTo)
		}
		if b.Points < prev.Points {
			add(i, "points must not decrease as values grow (%d after %d)", b.Points, prev.Points)
		}
	}
	return out
}

func isCustomRule(id string) bool {
	return strings.HasPrefix(id, "repo.") || strings.HasPrefix(id, "org.")
}
