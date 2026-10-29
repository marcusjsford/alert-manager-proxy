// Package model holds the v1 label types. The package and type names match
// Alertmanager 0.26's encoding/json error text, which is part of the contract.
package model

// LabelName is a label key.
type LabelName string

// LabelValue is a label value.
type LabelValue string

// LabelSet is a set of labels.
type LabelSet map[LabelName]LabelValue
