// Package types holds the v1 alert wire type. Its package/type names are
// deliberately the same as Alertmanager 0.26's so JSON decode errors match.
package types

import (
	"time"

	"alertmanager-proxy/internal/v1/model"
)

// Alert is the v1 alert shape accepted by POST /api/v1/alerts.
type Alert struct {
	Labels       model.LabelSet `json:"labels"`
	Annotations  model.LabelSet `json:"annotations,omitempty"`
	StartsAt     time.Time      `json:"startsAt,omitempty"`
	EndsAt       time.Time      `json:"endsAt,omitempty"`
	GeneratorURL string         `json:"generatorURL,omitempty"`
}
