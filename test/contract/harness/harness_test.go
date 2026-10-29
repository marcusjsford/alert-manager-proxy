package harness

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

var ref = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func TestExpand(t *testing.T) {
	vars := map[string]string{"sil1": "abc"}
	got := Expand(`${now} ${now+1h} ${now-10m} ${sil1} ${nope}`, ref, vars)
	want := "2026-10-05T12:00:00.000Z 2026-10-05T13:00:00.000Z 2026-10-05T11:50:00.000Z abc <missing:nope>"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func run(refTime time.Time, bodies ...string) Run {
	r := Run{Meta: Meta{RefTime: refTime}}
	for i, b := range bodies {
		r.Exchanges = append(r.Exchanges, Exchange{
			Step: string(rune('a' + i)), Endpoint: "GET /x",
			Response: Response{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: b},
		})
	}
	return r
}

func TestNormaliseMakesEquivalentRunsEqual(t *testing.T) {
	// Same logical content, different absolute times, IDs and peers, and
	// silencedBy in a different order.
	a := run(ref,
		`{"data":{"silenceId":"11111111-1111-4111-8111-111111111111"}}`,
		`{"data":{"silenceId":"22222222-2222-4222-8222-222222222222"}}`,
		`{"startsAt":"2026-10-05T12:00:03.123456789Z","endsAt":"2026-10-05T12:05:03Z",
		  "status":{"silencedBy":["11111111-1111-4111-8111-111111111111","22222222-2222-4222-8222-222222222222"]},
		  "versionInfo":{"version":"0.26.0"},"clusterStatus":{"name":"01AAA","peers":[{"name":"01AAA","address":"172.17.0.2:9094"}]}}`,
		`Error for 22222222-2222-4222-8222-222222222222`)
	later := ref.Add(26 * time.Hour)
	b := run(later,
		`{"data":{"silenceId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}}`,
		`{"data":{"silenceId":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}}`,
		`{"startsAt":"2026-10-06T14:00:01Z","endsAt":"2026-10-06T14:05:01.5Z",
		  "status":{"silencedBy":["bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"]},
		  "versionInfo":{"version":"0.34.1"},"clusterStatus":{"name":"01BBB","peers":[{"name":"01BBB","address":"10.0.0.9:9094"}]}}`,
		`Error for bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb`)
	for _, r := range Compare(a, b) {
		if len(r.Diffs) > 0 {
			t.Errorf("step %s: %v", r.Step, r.Diffs)
		}
	}
}

func TestNormaliseKeepsMeaningfulDifferences(t *testing.T) {
	cases := map[string][2]string{
		"endsAt offset":       {`{"endsAt":"2026-10-05T12:05:00Z"}`, `{"endsAt":"2026-10-05T13:00:00Z"}`},
		"null vs empty array": {`{"silencedBy":null}`, `{"silencedBy":[]}`},
		"null vs object":      {`{"annotations":null}`, `{"annotations":{}}`},
		"fingerprint":         {`{"fingerprint":"a965885991f6de03"}`, `{"fingerprint":"0000000000000000"}`},
		"extra field":         {`{"a":1}`, `{"a":1,"updatedAt":"2026-10-05T12:00:00Z"}`},
		"array order":         {`{"data":[{"n":"A"},{"n":"B"}]}`, `{"data":[{"n":"B"},{"n":"A"}]}`},
		"different uuid refs": {`{"x":"11111111-1111-4111-8111-111111111111","y":"11111111-1111-4111-8111-111111111111"}`, `{"x":"11111111-1111-4111-8111-111111111111","y":"22222222-2222-4222-8222-222222222222"}`},
		"plain text body":     {`Error getting silence: <nil>` + "\n", `{"status":"error"}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			res := Compare(run(ref, c[0]), run(ref, c[1]))
			if len(res[0].Diffs) == 0 {
				t.Fatalf("difference not detected between %s and %s", c[0], c[1])
			}
		})
	}
}

func TestCompareStatusAndHeaders(t *testing.T) {
	a, b := run(ref, `{}`), run(ref, `{}`)
	b.Exchanges[0].Response.Status = 501
	b.Exchanges[0].Response.Headers = map[string]string{"Content-Type": "text/plain"}
	diffs := Compare(a, b)[0].Diffs
	joined := strings.Join(diffs, "\n")
	if !strings.Contains(joined, "status: want 200, got 501") || !strings.Contains(joined, "header Content-Type") {
		t.Fatalf("diffs: %v", diffs)
	}
}

func TestReportVerdicts(t *testing.T) {
	results := []StepResult{
		{Step: "1", Endpoint: "pass"},
		{Step: "2", Endpoint: "fail", Diffs: []string{"x"}},
		{Step: "3", Endpoint: "xfail", Diffs: []string{"x"}},
		{Step: "4", Endpoint: "xpass"},
	}
	cases := []struct {
		name    string
		results []StepResult
		pending map[string]bool
		ok      bool
		verdict string
	}{
		{"pass", results[:1], nil, true, "PASS"},
		{"fail", results[1:2], nil, false, "FAIL"},
		{"xfail", results[2:3], map[string]bool{"xfail": true}, true, "XFAIL"},
		{"xpass is not ok", results[3:4], map[string]bool{"xpass": true}, false, "XPASS"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if ok := Report(&buf, c.results, c.pending, false); ok != c.ok {
				t.Errorf("ok = %v, want %v\n%s", ok, c.ok, buf.String())
			}
			if !strings.Contains(buf.String(), c.verdict) {
				t.Errorf("verdict %s missing:\n%s", c.verdict, buf.String())
			}
		})
	}
}

func TestNormaliseSeedsUUIDOrder(t *testing.T) {
	const (
		idA1 = "11111111-1111-4111-8111-111111111111"
		idA2 = "99999999-9999-4999-8999-999999999999"
		idB1 = "88888888-8888-4888-8888-888888888888"
		idB2 = "22222222-2222-4222-8222-222222222222"
	)
	a := run(ref, `{"status":{"silencedBy":["`+idA1+`","`+idA2+`"]}}`, `{"id":"`+idA2+`"}`)
	a.Meta.UUIDSeeds = []string{idA1, idA2}
	b := run(ref, `{"status":{"silencedBy":["`+idB1+`","`+idB2+`"]}}`, `{"id":"`+idB2+`"}`)
	b.Meta.UUIDSeeds = []string{idB1, idB2}
	for _, result := range Compare(a, b) {
		if len(result.Diffs) > 0 {
			t.Errorf("step %s: %v", result.Step, result.Diffs)
		}
	}
}
