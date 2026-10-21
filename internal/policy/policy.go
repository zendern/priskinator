// Package policy loads, validates, and layers priskinator policy: built-in defaults, org baseline, then repo policy, where a repo can loosen the defaults but only tighten the org baseline (ADR 0010).
package policy

import (
	_ "embed"
	"slices"
)

// Version is the policy schema version this build reads.
const Version = 1

// MaxPolicyBytes caps the size of one policy file.
const MaxPolicyBytes = 1 << 20

// Triggers the policy layer itself can fire (ADR 0003, ADR 0010).
const (
	TriggerConfigInvalid          = "config-invalid"
	TriggerPolicyWeakeningAttempt = "policy-weakening-attempt"
)

//go:embed policy.v1.schema.json
var schemaJSON []byte

//go:embed defaults.yaml
var defaultsYAML []byte

// SchemaJSON returns the published JSON Schema for policy version 1.
func SchemaJSON() []byte { return slices.Clone(schemaJSON) }

// Risk is an ordinal risk level.
type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

// Rank orders risks: low < medium < high. Unknown values rank highest, so a
// bad value can only make a verdict stricter.
func (r Risk) Rank() int {
	switch r {
	case RiskLow:
		return 0
	case RiskMedium:
		return 1
	default:
		return 2
	}
}

// MaxRisk returns the highest of the given risks, or low when none are given.
func MaxRisk(risks ...Risk) Risk {
	out := RiskLow
	for _, r := range risks {
		if r.Rank() > out.Rank() {
			out = r
		}
	}
	return out
}

// Policy is one policy layer, as written. Optional scalars are pointers so a
// layer can leave a field unset and inherit it.
type Policy struct {
	Version  int                 `yaml:"version"`
	Extends  string              `yaml:"extends"`
	Match    Match               `yaml:"match"`
	Limits   Limits              `yaml:"limits"`
	Triggers map[string]*Trigger `yaml:"triggers"`
	Scoring  Scoring             `yaml:"scoring"`
	AI       AI                  `yaml:"ai"`
}

type Match struct {
	CaseSensitive *bool `yaml:"case_sensitive"`
}

type Limits struct {
	MaxFileBytes    *int64 `yaml:"max_file_bytes"`
	MaxChangedFiles *int   `yaml:"max_changed_files"`
	MaxChangedLines *int   `yaml:"max_changed_lines"`
}

// Trigger configures one mandatory-review rule (ADR 0006).
type Trigger struct {
	Description string   `yaml:"description"`
	Enabled     *bool    `yaml:"enabled"`
	Paths       []string `yaml:"paths"`
	AddPaths    []string `yaml:"add_paths"`
	Reviewers   []string `yaml:"reviewers"`
	Risk        Risk     `yaml:"risk"`
}

// IsEnabled reports whether the rule is on. Rules are on unless disabled.
func (t *Trigger) IsEnabled() bool { return t.Enabled == nil || *t.Enabled }

type Scoring struct {
	Thresholds   *Thresholds `yaml:"thresholds"`
	Factors      Factors     `yaml:"factors"`
	Floors       []Floor     `yaml:"floors"`
	ExcludePaths []string    `yaml:"exclude_paths"`
}

// Thresholds are the total points at which the score tier becomes medium and high.
type Thresholds struct {
	Medium int `yaml:"medium"`
	High   int `yaml:"high"`
}

// Factors map each scoring factor's raw value to points (ADR 0007).
type Factors struct {
	Size            Bands `yaml:"size"`
	Files           Bands `yaml:"files"`
	Diffusion       Bands `yaml:"diffusion"`
	ComplexityDelta Bands `yaml:"complexity_delta"`
	TestSignal      Bands `yaml:"test_signal"`
}

// Bands are ordered by UpTo; the last band has no UpTo and covers the rest.
type Bands []Band

type Band struct {
	UpTo   *int `yaml:"up_to"`
	Points int  `yaml:"points"`
}

// Points returns the points for a raw value.
func (b Bands) Points(value int) int {
	for _, band := range b {
		if band.UpTo == nil || value <= *band.UpTo {
			return band.Points
		}
	}
	return 0
}

// Floor sets a minimum risk for changes to its paths.
type Floor struct {
	Description string   `yaml:"description"`
	Paths       []string `yaml:"paths"`
	Risk        Risk     `yaml:"risk"`
}

// AI configures the optional AI assessment (ADR 0008).
type AI struct {
	Enabled        *bool  `yaml:"enabled"`
	Required       *bool  `yaml:"required"`
	Provider       string `yaml:"provider"`
	Model          string `yaml:"model"`
	Endpoint       string `yaml:"endpoint"`
	MaxInputTokens int    `yaml:"max_input_tokens"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// IsEnabled reports whether the AI assessment is on. It is off unless enabled.
func (a AI) IsEnabled() bool { return a.Enabled != nil && *a.Enabled }

// IsRequired reports whether the AI assessment is mandatory, so a repo cannot
// turn it off.
func (a AI) IsRequired() bool { return a.Required != nil && *a.Required }

// Defaults returns a fresh copy of the built-in defaults (ADR 0010, layer 1).
// Tests guarantee the embedded defaults validate.
func Defaults() *Policy {
	p, err := Parse(defaultsYAML)
	if err != nil {
		panic("built-in policy defaults: " + err.Error())
	}
	return p
}
