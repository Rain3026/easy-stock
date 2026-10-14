package foundation

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// MarketBreadth counts only traded SH/SZ A-share rows from a complete response.
type MarketBreadth struct {
	Date                 string     `json:"date"`
	Up                   int        `json:"up"`
	Down                 int        `json:"down"`
	Flat                 int        `json:"flat"`
	Source               string     `json:"source"`
	At                   string     `json:"at"`
	Coverage             int        `json:"coverage,omitempty"`
	Excluded             int        `json:"excluded,omitempty"`
	Meta                 SourceMeta `json:"meta"`
	LatestAt             string     `json:"latest_at,omitempty"`
	UniverseHash         string     `json:"universe_hash,omitempty"`
	ActiveHash           string     `json:"active_hash,omitempty"`
	OtherMarketsExcluded int        `json:"other_markets_excluded,omitempty"`
	// Full public evidence is emitted by the validation command only, not by
	// every dashboard response or historical research record.
	Members []BreadthMember `json:"-"`
}

type BreadthMember struct {
	Symbol     string  `json:"symbol"`
	At         string  `json:"at,omitempty"`
	Amount     float64 `json:"amount_cny"`
	Change     float64 `json:"change"`
	ChangeUnit string  `json:"change_unit"`
}

func IsSHSZAShare(code string) bool {
	if len(code) != 9 {
		return false
	}
	for _, c := range code[:6] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return strings.HasSuffix(code, ".SH") && code[0] == '6' || strings.HasSuffix(code, ".SZ") && (code[0] == '0' || code[0] == '3')
}

func AttachBreadthMembers(b *MarketBreadth, members []BreadthMember) {
	sort.Slice(members, func(i, j int) bool { return members[i].Symbol < members[j].Symbol })
	all, active := []string{}, []string{}
	for _, m := range members {
		all = append(all, m.Symbol)
		if m.Amount > 0 {
			active = append(active, m.Symbol)
		}
	}
	b.Members = members
	b.Coverage = len(members)
	b.Excluded = len(members) - len(active)
	b.UniverseHash = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(all, "\n"))))
	b.ActiveHash = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(active, "\n"))))
}
