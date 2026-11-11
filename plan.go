package main

import "fmt"

// planShare estimates how much of a $20 subscription's limits one check used.
//
// Subscriptions only show limits as percentages, so these are calibrations,
// not published numbers. Measured on 2026-11-12 from one PC's own logs against
// the percentages the CLIs report (Claude Code's rate_limit_event, Codex's
// rate_limits):
//   - Claude: Max 5x spent about $1.12 of API-equivalent cost per 1% of the
//     5-hour limit and $10.8 per 1% of the weekly limit. Pro ($20) has a fifth
//     of Max 5x, so $0.224 and $2.16. Usage from other devices counts toward
//     the same limits but is not in the logs, so these lean high (safe side).
//   - Codex: Plus ($20), weighted tokens per 1%, where weighted tokens are
//     uncached input, plus a tenth of cached input, plus eight times output:
//     about 72k for 5 hours and 143k for the week.
//
// Antigravity and the API-key adapters return ok=false: no calibration yet,
// and API keys are billed per use instead of drawing from a plan.
var planCalib = map[string]struct{ per5h, perWeek float64 }{
	"claude": {0.224, 2.16},   // API-equivalent USD per 1%
	"codex":  {72000, 143000}, // weighted tokens per 1%
}

func planShare(r Run) (fiveH, week float64, ok bool) {
	cal, found := planCalib[r.Agent]
	if !found {
		return 0, 0, false
	}
	var amount float64
	for _, c := range r.Calls {
		switch r.Agent {
		case "claude":
			amount += c.CostUSD
		case "codex":
			amount += float64(c.TokensIn-c.TokensCached) + 0.1*float64(c.TokensCached) + 8*float64(c.TokensOut)
		}
	}
	if amount <= 0 {
		return 0, 0, false
	}
	return amount / cal.per5h, amount / cal.perWeek, true
}

// pct formats a small percentage for the dashboard: "about 0.5%", "under 0.1%".
func pct(lang string, v float64) string {
	if v < 0.1 {
		return tr(lang, "pct_tiny")
	}
	s := fmt.Sprintf("%.1f%%", v)
	if v >= 10 {
		s = fmt.Sprintf("%.0f%%", v)
	}
	return trf(lang, "pct_about", s)
}
