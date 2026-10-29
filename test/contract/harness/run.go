package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Exchange is one recorded or live request/response.
type Exchange struct {
	Step     string   `json:"step"`
	Endpoint string   `json:"endpoint"`
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

// Request is what the harness sent, after expansion.
type Request struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

// Response is what the target returned. Body is raw text; it is parsed (if
// JSON) only when normalising.
type Response struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// Meta describes a run.
type Meta struct {
	Target              string    `json:"target"`
	AlertmanagerVersion string    `json:"alertmanagerVersion"`
	RefTime             time.Time `json:"refTime"`
	ScenarioSHA256      string    `json:"scenarioSha256"`
	// UUIDSeeds lists UUIDs in scenario execution order, including uncompared
	// setup responses. The normaliser uses it so placeholders follow creation
	// order rather than Alertmanager's random map iteration order.
	UUIDSeeds []string `json:"uuidSeeds,omitempty"`
}

// Run is the result of executing a scenario.
type Run struct {
	Meta      Meta
	Exchanges []Exchange // compared steps only, in order
}

// ComparedHeaders are the response headers that are part of the contract.
// Date and Content-Length are excluded: volatile or implied by the body.
var ComparedHeaders = []string{
	"Content-Type",
	"Cache-Control",
	"Access-Control-Allow-Origin",
	"Access-Control-Allow-Methods",
	"Access-Control-Allow-Headers",
	"Access-Control-Expose-Headers",
}

// Runner executes a scenario against a base URL.
type Runner struct {
	Target string
	Client *http.Client
	// Strict makes wait_for timeouts fatal (recording), rather than a warning
	// (verifying, where an unimplemented endpoint can legitimately stall one).
	Strict bool
	Logf   func(format string, args ...any)
}

// WaitReady polls GET /-/ready until it returns 200.
func (r *Runner) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := r.do(ctx, http.MethodGet, "/-/ready", nil, "")
		if err == nil && resp.Status == http.StatusOK {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("%s not ready after %s: %w", r.Target, timeout, err)
			}
			return fmt.Errorf("%s not ready after %s: last status %d", r.Target, timeout, resp.Status)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Execute runs every step in order.
func (r *Runner) Execute(ctx context.Context, sc Scenario) (Run, error) {
	ref := time.Now().UTC()
	vars := map[string]string{}
	run := Run{Meta: Meta{Target: r.Target, RefTime: ref, ScenarioSHA256: sc.SHA256}}
	seenUUIDs := map[string]bool{}

	for _, st := range sc.Steps {
		path := Expand(st.Path, ref, vars)
		body := Expand(st.Body, ref, vars)
		if st.WaitFor != nil {
			if err := r.wait(ctx, st, path); err != nil {
				if r.Strict {
					return run, err
				}
				r.Logf("warning: %v", err)
			}
			continue
		}
		resp, err := r.do(ctx, st.Method, path, st.Headers, body)
		if err != nil {
			return run, fmt.Errorf("step %q: %w", st.Name, err)
		}
		for _, id := range uuidRe.FindAllString(resp.Body, -1) {
			if !seenUUIDs[id] {
				seenUUIDs[id] = true
				run.Meta.UUIDSeeds = append(run.Meta.UUIDSeeds, id)
			}
		}
		for name, jpath := range st.Capture {
			if v, ok := lookup(resp.Body, jpath); ok {
				vars[name] = v
			} else {
				r.Logf("step %q: capture %s=%s not found in response", st.Name, name, jpath)
			}
		}
		if st.Compared() {
			run.Exchanges = append(run.Exchanges, Exchange{
				Step: st.Name, Endpoint: st.Endpoint,
				Request:  Request{Method: st.Method, Path: path, Body: body},
				Response: resp,
			})
		}
	}

	if resp, err := r.do(ctx, http.MethodGet, "/api/v2/status", nil, ""); err == nil {
		if v, ok := lookup(resp.Body, "versionInfo.version"); ok {
			run.Meta.AlertmanagerVersion = v
		}
	}
	return run, nil
}

func (r *Runner) wait(ctx context.Context, st Step, path string) error {
	timeout := st.WaitFor.Timeout
	if timeout == 0 {
		// Recording waits patiently; verifying an endpoint that isn't
		// translated yet would otherwise spend the full wait every run.
		timeout = 15 * time.Second
		if !r.Strict {
			timeout = 5 * time.Second
		}
	}
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		resp, err := r.do(ctx, st.Method, path, st.Headers, "")
		if err == nil {
			last = resp.Body
			okContains := st.WaitFor.Contains == "" || strings.Contains(resp.Body, st.WaitFor.Contains)
			okNot := st.WaitFor.NotContains == "" || !strings.Contains(resp.Body, st.WaitFor.NotContains)
			if okContains && okNot {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if len(last) > 300 {
		last = last[:300] + "…"
	}
	return fmt.Errorf("step %q: condition not met within %s; last body: %s", st.Name, timeout, last)
}

func (r *Runner) do(ctx context.Context, method, path string, headers map[string]string, body string) (Response, error) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(r.Target, "/")+path, rdr)
	if err != nil {
		return Response{}, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	hdrs := map[string]string{}
	for _, h := range ComparedHeaders {
		if v := resp.Header.Get(h); v != "" {
			hdrs[h] = v
		}
	}
	return Response{Status: resp.StatusCode, Headers: hdrs, Body: string(b)}, nil
}

// lookup follows a dotted path through a JSON document to a scalar.
func lookup(body, path string) (string, bool) {
	var v any
	if json.Unmarshal([]byte(body), &v) != nil {
		return "", false
	}
	for _, key := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return "", false
		}
		if v, ok = m[key]; !ok {
			return "", false
		}
	}
	switch t := v.(type) {
	case string:
		return t, true
	case nil:
		return "", false
	default:
		return fmt.Sprint(t), true
	}
}

// Save writes a run as fixtures: meta.json plus one NNN-step.json per exchange.
func Save(run Run, dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "meta.json"), run.Meta); err != nil {
		return err
	}
	for i, ex := range run.Exchanges {
		if err := writeJSON(filepath.Join(dir, fmt.Sprintf("%03d-%s.json", i+1, ex.Step)), ex); err != nil {
			return err
		}
	}
	return nil
}

// Load reads fixtures written by Save.
func Load(dir string) (Run, error) {
	var run Run
	if err := readJSON(filepath.Join(dir, "meta.json"), &run.Meta); err != nil {
		return run, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "[0-9][0-9][0-9]-*.json"))
	if err != nil {
		return run, err
	}
	for _, f := range files { // Glob returns lexical order == step order
		var ex Exchange
		if err := readJSON(f, &ex); err != nil {
			return run, err
		}
		run.Exchanges = append(run.Exchanges, ex)
	}
	return run, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path) //nolint:gosec // G304: fixture paths come from the operator
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
