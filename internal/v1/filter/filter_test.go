package filter_test

import (
	"strings"
	"testing"

	"alertmanager-proxy/internal/v1/filter"

	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestParseAndValues(t *testing.T) {
	cases := []struct {
		name, input string
		values      []string
	}{
		{"braces", `{team="db"}`, []string{`team="db"`}},
		{"no braces", `team="db"`, []string{`team="db"`}},
		{"equal and not equal", `{team="db",severity!="critical"}`, []string{`team="db"`, `severity!="critical"`}},
		{"regexp and not regexp", `{team=~"d.*",severity!~"crit.*"}`, []string{`team=~"d.*"`, `severity!~"crit.*"`}},
		{"quoted comma", `{summary="a,b",team="db"}`, []string{`summary="a,b"`, `team="db"`}},
		{"escaped quote", `{summary="She said: \"Hi, ladies!\""}`, []string{`summary="She said: \"Hi, ladies!\""`}},
		{"escaped newline", `{summary="line\nbreak"}`, []string{`summary="line\nbreak"`}},
		{"trailing comma", `{team="db",}`, []string{`team="db"`}},
		{"empty set", `{}`, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matchers, err := filter.Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			values := filter.Values(matchers)
			if strings.Join(values, "|") != strings.Join(tc.values, "|") {
				t.Errorf("values = %q, want %q", values, tc.values)
			}
		})
	}
}

func TestParseMatcherTypes(t *testing.T) {
	matchers, err := filter.Parse(`{a="b",c!="d",e=~"f",g!~"h"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []labels.MatchType{labels.MatchEqual, labels.MatchNotEqual, labels.MatchRegexp, labels.MatchNotRegexp}
	if len(matchers) != len(want) {
		t.Fatalf("matchers = %d", len(matchers))
	}
	for i, matcher := range matchers {
		if matcher.Type != want[i] {
			t.Errorf("matcher %d type = %v, want %v", i, matcher.Type, want[i])
		}
	}
}

func TestParseInvalid(t *testing.T) {
	for _, input := range []string{`{bad`, `bad`, `{a!~"("}`} {
		t.Run(input, func(t *testing.T) {
			if _, err := filter.Parse(input); err == nil {
				t.Fatalf("%q parsed without error", input)
			}
		})
	}
	if _, err := filter.Parse(`{bad`); err == nil || err.Error() != "bad matcher format: bad" {
		t.Errorf("error = %v, want bad matcher format: bad", err)
	}
}
