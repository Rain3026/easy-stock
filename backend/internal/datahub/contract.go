package datahub

import (
	"fmt"
	"math"
)

// AmountCNY requires an explicit upstream unit. Missing/unknown is an error;
// magnitude guessing could turn a thousand-yuan bar into a thousandfold error.
func AmountCNY(value float64, unit string) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, fmt.Errorf("invalid turnover")
	}
	factor := 0.0
	switch unit {
	case "CNY":
		factor = 1
	case "CNY_thousand":
		factor = 1000
	case "CNY_100m":
		factor = 1e8
	default:
		return 0, fmt.Errorf("unknown turnover unit %q", unit)
	}
	n := value * factor
	if math.IsInf(n, 0) {
		return 0, fmt.Errorf("turnover overflow")
	}
	return n, nil
}

// CanSubstitute deliberately excludes a source whose classification/frequency
// only provides a degraded approximation of the requested contract.
func CanSubstitute(primary Candidate, backup Candidate) bool {
	return primary.Family != "" && backup.Family != "" && primary.Family != backup.Family && backup.Equivalent && backup.State == "implemented"
}
