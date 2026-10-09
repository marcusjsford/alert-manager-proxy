package harness

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"
)

// Normalised is an exchange's response with volatile values replaced, ready
// to compare.
type Normalised struct {
	Step     string
	Endpoint string
	Status   int
	Headers  map[string]string
	Body     any // decoded JSON with placeholders, or the raw string if not JSON
}

// setKeys are arrays whose order carries no meaning: Alertmanager's order
// for them follows the (random) silence IDs and differs between instances,
// found by the determinism check. They are compared as sets.
var setKeys = map[string]bool{"silencedBy": true, "inhibitedBy": true}

var uuidRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// normaliser carries per-run state so the same UUID gets the same
// placeholder in every step of a run.
type normaliser struct {
	ref   time.Time
	uuids map[string]string
}

// Normalise replaces everything that legitimately differs between two runs:
//
//   - RFC3339 timestamps become their offset from the run's reference time,
//     rounded to the minute ("<now+60m>"). Offsets are kept rather than
//     masked so that, e.g., a default endsAt of now+resolve_timeout is still
//     checked. Sub-second precision is deliberately not compared.
//   - UUIDs (silence IDs) become <uuid:N> by first appearance in the run, so
//     references between steps (silencedBy -> id) stay checkable.
//   - silencedBy/inhibitedBy are sorted (see setKeys).
//   - versionInfo is masked whole; cluster/peer names and addresses are masked.
//
// Fingerprints are kept: they are a hash of the label set alone.
func Normalise(run Run) []Normalised {
	n := normaliser{ref: run.Meta.RefTime, uuids: map[string]string{}}
	out := make([]Normalised, 0, len(run.Exchanges))
	for _, ex := range run.Exchanges {
		var body any
		if json.Unmarshal([]byte(ex.Response.Body), &body) == nil && ex.Response.Body != "" {
			body = n.value(body, "")
		} else {
			body = n.text(ex.Response.Body)
		}
		out = append(out, Normalised{
			Step: ex.Step, Endpoint: ex.Endpoint,
			Status: ex.Response.Status, Headers: ex.Response.Headers, Body: body,
		})
	}
	return out
}

func (n *normaliser) value(v any, parentKey string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			switch {
			case k == "versionInfo":
				out[k] = "<versionInfo>"
			case (parentKey == "clusterStatus" || parentKey == "cluster" || parentKey == "peers") && k == "name":
				out[k] = "<peer-name>"
			case (parentKey == "peers") && k == "address":
				out[k] = "<peer-address>"
			default:
				out[k] = n.value(child, k)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			// Array elements inherit the array's key so rules like "peers" apply.
			out[i] = n.value(child, parentKey)
		}
		if setKeys[parentKey] {
			sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]) < fmt.Sprint(out[j]) })
		}
		return out
	case string:
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return n.timestamp(ts)
		}
		return n.text(t)
	default:
		return v
	}
}

func (n *normaliser) text(s string) string {
	return uuidRe.ReplaceAllStringFunc(s, func(id string) string {
		if p, ok := n.uuids[id]; ok {
			return p
		}
		p := fmt.Sprintf("<uuid:%d>", len(n.uuids)+1)
		n.uuids[id] = p
		return p
	})
}

func (n *normaliser) timestamp(t time.Time) string {
	m := int(math.Round(t.Sub(n.ref).Minutes()))
	switch {
	case m == 0:
		return "<now>"
	case m > 0:
		return fmt.Sprintf("<now+%dm>", m)
	default:
		return fmt.Sprintf("<now%dm>", m)
	}
}
