// Package harness drives the v1 contract scenario against any Alertmanager-
// compatible base URL, records responses as fixtures, and compares normalised
// responses between a recording and a live run. It speaks only HTTP to its
// target, so it accepts any implementation of the proxy (ADR decision 1).
package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Scenario is the ordered list of steps in scenario.yaml.
type Scenario struct {
	Steps  []Step `yaml:"steps"`
	SHA256 string `yaml:"-"`
}

// Step is one HTTP exchange, or a wait.
type Step struct {
	Name string `yaml:"name"`
	// Endpoint groups steps in the report and keys the pending list. Steps
	// with endpoint "setup" are never compared.
	Endpoint string            `yaml:"endpoint"`
	Method   string            `yaml:"method"`
	Path     string            `yaml:"path"`
	Headers  map[string]string `yaml:"headers"`
	Body     string            `yaml:"body"`
	// Capture stores a value from the JSON response for later ${var} use,
	// keyed by variable name, valued by a dotted path (e.g. data.silenceId).
	Capture map[string]string `yaml:"capture"`
	// WaitFor polls Path until the condition holds, then the step is done.
	WaitFor *WaitFor `yaml:"wait_for"`
}

// WaitFor is a polling condition on the raw response body.
type WaitFor struct {
	Contains    string        `yaml:"contains"`
	NotContains string        `yaml:"not_contains"`
	Timeout     time.Duration `yaml:"timeout"`
}

// Compared reports whether the step's response is part of the contract.
func (s Step) Compared() bool { return s.Endpoint != "setup" && s.WaitFor == nil }

// LoadScenario reads and validates a scenario file.
func LoadScenario(path string) (Scenario, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path is the operator's scenario file
	if err != nil {
		return Scenario{}, err
	}
	var sc Scenario
	if err := yaml.Unmarshal(raw, &sc); err != nil {
		return Scenario{}, fmt.Errorf("parse %s: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	sc.SHA256 = hex.EncodeToString(sum[:])
	seen := map[string]bool{}
	for i, st := range sc.Steps {
		switch {
		case st.Name == "":
			return Scenario{}, fmt.Errorf("step %d: name required", i)
		case seen[st.Name]:
			return Scenario{}, fmt.Errorf("step %q: duplicate name", st.Name)
		case st.Endpoint == "":
			return Scenario{}, fmt.Errorf("step %q: endpoint required", st.Name)
		case st.Method == "" || st.Path == "":
			return Scenario{}, fmt.Errorf("step %q: method and path required", st.Name)
		}
		seen[st.Name] = true
	}
	return sc, nil
}

var varRe = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)([+-][0-9]+[smh])?\}`)

// Expand substitutes ${now}, ${now+1h}, ${now-10m} (relative to ref) and
// captured ${var}s. An unknown variable expands to "<missing:var>" so the
// step still runs and fails visibly rather than aborting the scenario.
func Expand(s string, ref time.Time, vars map[string]string) string {
	return varRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := varRe.FindStringSubmatch(m)
		name, offset := parts[1], parts[2]
		if name == "now" {
			t := ref
			if offset != "" {
				d, err := time.ParseDuration(strings.TrimPrefix(offset, "+"))
				if err != nil {
					return m
				}
				t = t.Add(d)
			}
			return t.UTC().Format("2006-01-02T15:04:05.000Z")
		}
		if v, ok := vars[name]; ok && offset == "" {
			return v
		}
		return "<missing:" + name + ">"
	})
}
