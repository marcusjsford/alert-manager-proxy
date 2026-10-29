// Package filter parses the v1 filter= matcher-set with Alertmanager's own
// parser, so quoting, escaping and error strings match the recorded contract.
package filter

import (
	"github.com/prometheus/alertmanager/pkg/labels"
)

// Parse parses one v1 filter value. Leading/trailing braces are optional.
func Parse(value string) ([]*labels.Matcher, error) {
	return labels.ParseMatchers(value)
}

// Values converts matchers to the repeated v2 filter parameter form. The v1
// matcher-set is always sent as one parameter per matcher; v2 rejects braces.
func Values(matchers []*labels.Matcher) []string {
	values := make([]string, len(matchers))
	for i, matcher := range matchers {
		values[i] = matcher.String()
	}
	return values
}
