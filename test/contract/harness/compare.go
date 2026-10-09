package harness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
)

// StepResult is the comparison of one step.
type StepResult struct {
	Step     string
	Endpoint string
	Diffs    []string
}

// Compare diffs two runs step by step, matched by step name in want's order.
func Compare(want, got Run) []StepResult {
	gotByStep := map[string]Normalised{}
	for _, g := range Normalise(got) {
		gotByStep[g.Step] = g
	}
	var results []StepResult
	for _, w := range Normalise(want) {
		res := StepResult{Step: w.Step, Endpoint: w.Endpoint}
		g, ok := gotByStep[w.Step]
		if !ok {
			res.Diffs = []string{"step missing from run"}
			results = append(results, res)
			continue
		}
		if w.Status != g.Status {
			res.Diffs = append(res.Diffs, fmt.Sprintf("status: want %d, got %d", w.Status, g.Status))
		}
		for _, h := range ComparedHeaders {
			if w.Headers[h] != g.Headers[h] {
				res.Diffs = append(res.Diffs, fmt.Sprintf("header %s: want %q, got %q", h, w.Headers[h], g.Headers[h]))
			}
		}
		res.Diffs = append(res.Diffs, diff("body", w.Body, g.Body)...)
		results = append(results, res)
	}
	return results
}

// diff returns one line per differing leaf, addressed by a JSON-ish path.
func diff(path string, want, got any) []string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: want object, got %s", path, show(got))}
		}
		keys := map[string]bool{}
		for k := range w {
			keys[k] = true
		}
		for k := range g {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		var out []string
		for _, k := range sorted {
			wv, wok := w[k]
			gv, gok := g[k]
			switch {
			case !gok:
				out = append(out, fmt.Sprintf("%s.%s: missing (want %s)", path, k, show(wv)))
			case !wok:
				out = append(out, fmt.Sprintf("%s.%s: unexpected (got %s)", path, k, show(gv)))
			default:
				out = append(out, diff(path+"."+k, wv, gv)...)
			}
		}
		return out
	case []any:
		g, ok := got.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: want array, got %s", path, show(got))}
		}
		if len(w) != len(g) {
			return []string{fmt.Sprintf("%s: want %d elements, got %d: want %s, got %s", path, len(w), len(g), show(w), show(g))}
		}
		var out []string
		for i := range w {
			out = append(out, diff(fmt.Sprintf("%s[%d]", path, i), w[i], g[i])...)
		}
		return out
	default:
		if !reflect.DeepEqual(want, got) {
			return []string{fmt.Sprintf("%s: want %s, got %s", path, show(want), show(got))}
		}
		return nil
	}
}

func show(v any) string {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	s := strings.TrimSuffix(buf.String(), "\n")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// LoadPending reads endpoint labels expected to fail, one per line; blank
// lines and # comments are ignored.
func LoadPending(path string) (map[string]bool, error) {
	pending := map[string]bool{}
	if path == "" {
		return pending, nil
	}
	f, err := os.Open(path) //nolint:gosec // G304: operator-supplied list
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			pending[line] = true
		}
	}
	return pending, sc.Err()
}

// Verdicts for an endpoint.
const (
	Pass  = "PASS"
	Fail  = "FAIL"
	XFail = "XFAIL" // pending and failing: expected
	XPass = "XPASS" // pending but passing: the pending list is stale
)

// Report prints a per-endpoint table (and, if verbose, each diff) and
// returns true when the run is acceptable: no FAIL and no XPASS.
func Report(w io.Writer, results []StepResult, pending map[string]bool, verbose bool) bool {
	type agg struct{ passed, total int }
	var order []string
	byEndpoint := map[string]*agg{}
	for _, r := range results {
		a, ok := byEndpoint[r.Endpoint]
		if !ok {
			a = &agg{}
			byEndpoint[r.Endpoint] = a
			order = append(order, r.Endpoint)
		}
		a.total++
		if len(r.Diffs) == 0 {
			a.passed++
		}
	}

	ok := true
	_, _ = fmt.Fprintf(w, "%-34s %-6s %s\n", "ENDPOINT", "STEPS", "VERDICT")
	for _, ep := range order {
		a := byEndpoint[ep]
		verdict := Pass
		switch {
		case a.passed == a.total && pending[ep]:
			verdict, ok = XPass, false
		case a.passed < a.total && pending[ep]:
			verdict = XFail
		case a.passed < a.total:
			verdict, ok = Fail, false
		}
		_, _ = fmt.Fprintf(w, "%-34s %2d/%-3d %s\n", ep, a.passed, a.total, verdict)
	}
	for ep := range pending {
		if _, seen := byEndpoint[ep]; !seen {
			_, _ = fmt.Fprintf(w, "warning: pending endpoint %q matches no scenario step\n", ep)
		}
	}

	for _, r := range results {
		if len(r.Diffs) == 0 || (pending[r.Endpoint] && !verbose) {
			continue
		}
		_, _ = fmt.Fprintf(w, "\n--- %s (%s)\n", r.Step, r.Endpoint)
		for _, d := range r.Diffs {
			_, _ = fmt.Fprintf(w, "    %s\n", d)
		}
	}
	return ok
}
