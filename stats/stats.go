package stats

import (
	"sort"
	"time"

	"claude_tray_usage/parser"
)

// Period filters entries by time window.
type Period int

const (
	PeriodToday Period = iota
	PeriodWeek
	PeriodMonth
	PeriodAllTime
)

func (p Period) String() string {
	switch p {
	case PeriodToday:
		return "Today"
	case PeriodWeek:
		return "This Week"
	case PeriodMonth:
		return "This Month"
	default:
		return "All Time"
	}
}

// Summary holds aggregated token counts.
type Summary struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	WebSearchRequests   int64
	WebFetchRequests    int64
	Messages            int64
	EstimatedCostUSD    float64
}

// ProjectSummary is a per-project breakdown.
type ProjectSummary struct {
	Name    string
	Summary Summary
}

// ModelSummary is a per-model breakdown.
type ModelSummary struct {
	Name    string
	Summary Summary
}

// DailySummary is a per-day breakdown.
type DailySummary struct {
	Date    time.Time
	Summary Summary
}

// Report is the full computed report for a given period.
type Report struct {
	Period     Period
	Total      Summary
	Projects   []ProjectSummary
	Models     []ModelSummary
	Daily      []DailySummary
	AllEntries []parser.UsageEntry // raw entries for cross-period queries in the UI
}

// Compute builds a Report from raw entries for the given period.
func Compute(entries []parser.UsageEntry, period Period) Report {
	filtered := filterByPeriod(entries, period)

	report := Report{Period: period, AllEntries: entries}
	projectMap := map[string]*Summary{}
	modelMap := map[string]*Summary{}
	dailyMap := map[string]*Summary{}

	for _, e := range filtered {
		add(&report.Total, e)

		if _, ok := projectMap[e.Project]; !ok {
			projectMap[e.Project] = &Summary{}
		}
		add(projectMap[e.Project], e)

		model := e.Model
		if model == "" {
			model = "unknown"
		}
		if _, ok := modelMap[model]; !ok {
			modelMap[model] = &Summary{}
		}
		add(modelMap[model], e)

		day := e.Timestamp.Format("2006-01-02")
		if _, ok := dailyMap[day]; !ok {
			dailyMap[day] = &Summary{}
		}
		add(dailyMap[day], e)
	}

	for name, s := range projectMap {
		s.EstimatedCostUSD = estimateCost(s)
		report.Projects = append(report.Projects, ProjectSummary{Name: name, Summary: *s})
	}
	sort.Slice(report.Projects, func(i, j int) bool {
		return report.Projects[i].Summary.EstimatedCostUSD > report.Projects[j].Summary.EstimatedCostUSD
	})

	for name, s := range modelMap {
		s.EstimatedCostUSD = estimateCost(s)
		report.Models = append(report.Models, ModelSummary{Name: name, Summary: *s})
	}
	sort.Slice(report.Models, func(i, j int) bool {
		return report.Models[i].Summary.EstimatedCostUSD > report.Models[j].Summary.EstimatedCostUSD
	})

	for dayStr, s := range dailyMap {
		t, _ := time.Parse("2006-01-02", dayStr)
		s.EstimatedCostUSD = estimateCost(s)
		report.Daily = append(report.Daily, DailySummary{Date: t, Summary: *s})
	}
	sort.Slice(report.Daily, func(i, j int) bool {
		return report.Daily[i].Date.After(report.Daily[j].Date)
	})

	report.Total.EstimatedCostUSD = estimateCost(&report.Total)
	return report
}

func add(s *Summary, e parser.UsageEntry) {
	s.InputTokens += e.InputTokens
	s.OutputTokens += e.OutputTokens
	s.CacheCreationTokens += e.CacheCreationTokens
	s.CacheReadTokens += e.CacheReadTokens
	s.WebSearchRequests += e.WebSearchRequests
	s.WebFetchRequests += e.WebFetchRequests
	s.Messages++
}

func filterByPeriod(entries []parser.UsageEntry, period Period) []parser.UsageEntry {
	if period == PeriodAllTime {
		return entries
	}
	now := time.Now()
	var cutoff time.Time
	switch period {
	case PeriodToday:
		cutoff = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case PeriodWeek:
		cutoff = now.AddDate(0, 0, -7)
	case PeriodMonth:
		cutoff = now.AddDate(0, -1, 0)
	}
	var out []parser.UsageEntry
	for _, e := range entries {
		if e.Timestamp.After(cutoff) {
			out = append(out, e)
		}
	}
	return out
}

// estimateCost uses approximate Anthropic pricing per million tokens.
// Claude Sonnet 4.x: $3/M input, $15/M output, $3.75/M cache write, $0.30/M cache read.
// We use Sonnet pricing as a rough default for all models.
func estimateCost(s *Summary) float64 {
	const (
		inputPricePer1M      = 3.00
		outputPricePer1M     = 15.00
		cacheWritePricePer1M = 3.75
		cacheReadPricePer1M  = 0.30
	)
	cost := float64(s.InputTokens)/1_000_000*inputPricePer1M +
		float64(s.OutputTokens)/1_000_000*outputPricePer1M +
		float64(s.CacheCreationTokens)/1_000_000*cacheWritePricePer1M +
		float64(s.CacheReadTokens)/1_000_000*cacheReadPricePer1M
	return cost
}

// RollingUsage holds token/cost usage over a rolling time window.
type RollingUsage struct {
	Window       time.Duration
	TotalTokens  int64
	CostUSD      float64
	Messages     int64
	WindowStart  time.Time
	WindowEnd    time.Time
}

// ComputeRolling returns usage for a rolling window ending now.
func ComputeRolling(entries []parser.UsageEntry, window time.Duration) RollingUsage {
	now := time.Now()
	cutoff := now.Add(-window)
	r := RollingUsage{
		Window:      window,
		WindowStart: cutoff,
		WindowEnd:   now,
	}
	var s Summary
	for _, e := range entries {
		if e.Timestamp.After(cutoff) {
			add(&s, e)
		}
	}
	s.EstimatedCostUSD = estimateCost(&s)
	r.TotalTokens = s.InputTokens + s.OutputTokens + s.CacheCreationTokens + s.CacheReadTokens
	r.CostUSD = s.EstimatedCostUSD
	r.Messages = s.Messages
	return r
}

// DayBoundaryReset returns the time of the next midnight reset (local time).
func DayBoundaryReset() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}

// MonthBoundaryReset returns the first day of next month at midnight.
func MonthBoundaryReset() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
}
