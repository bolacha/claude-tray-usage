package ui

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"claude_tray_usage/config"
	"claude_tray_usage/stats"
)

var (
	colorAccent  = color32(0xCC, 0x6B, 0x2C, 0xFF) // Anthropic orange
	colorMuted   = color32(0x88, 0x88, 0x88, 0xFF)
	colorSuccess = color32(0x34, 0xC7, 0x59, 0xFF)
	colorWarn    = color32(0xFF, 0x9F, 0x0A, 0xFF)
	colorDanger  = color32(0xFF, 0x3B, 0x30, 0xFF)
)

// RefreshFunc provides fresh data to the UI.
type RefreshFunc func() (stats.Report, []stats.RollingUsage)

// App holds the Fyne application state.
type App struct {
	fyneApp fyne.App
	window  fyne.Window

	currentPeriod stats.Period
	report        stats.Report
	rolling       []stats.RollingUsage
	cfg           config.Config
	onRefresh     RefreshFunc

	// hero widgets
	costText    *canvas.Text
	periodLabel *canvas.Text
	msgsLabel   *canvas.Text
	inputChip   *metricChip
	outputChip  *metricChip
	cacheWChip  *metricChip
	cacheRChip  *metricChip
	updatedText *canvas.Text

	// tables
	projectTable *widget.Table
	modelTable   *widget.Table
	dailyTable   *widget.Table

	// limits tab
	limitsContainer *fyne.Container
}

// New creates a new UI app.
func New(fyneApp fyne.App, onRefresh RefreshFunc) *App {
	return &App{
		fyneApp:       fyneApp,
		currentPeriod: stats.PeriodToday,
		onRefresh:     onRefresh,
		cfg:           config.Load(),
	}
}

// ShowWindow creates and shows the main stats window.
func (a *App) ShowWindow() {
	if a.window != nil {
		a.window.Show()
		a.window.RequestFocus()
		return
	}
	a.window = a.fyneApp.NewWindow("Claude Usage")
	a.window.SetCloseIntercept(func() { a.window.Hide() })
	a.window.Resize(fyne.NewSize(740, 580))
	a.window.SetContent(a.buildContent())
	a.refresh()
	a.window.Show()
}

// Refresh is the public entrypoint called by the background ticker.
func (a *App) Refresh() {
	if a.window != nil {
		a.refresh()
	}
}

// TrayLabel returns a short string for the tray tooltip.
func (a *App) TrayLabel() string {
	return fmt.Sprintf("Claude today: $%.4f · %d messages", a.report.Total.EstimatedCostUSD, a.report.Total.Messages)
}

// ── Build ──────────────────────────────────────────────────────────────────

func (a *App) buildContent() fyne.CanvasObject {
	// Hero widgets — must exist before periodSelect fires onChange.
	a.costText = newText("$0.0000", 30, true)
	a.costText.Color = colorAccent
	a.periodLabel = newText("Today", 11, false)
	a.periodLabel.Color = colorMuted
	a.msgsLabel = newText("0 messages", 11, false)
	a.msgsLabel.Color = colorMuted
	a.updatedText = newText("", 10, false)
	a.updatedText.Color = colorMuted

	a.inputChip = newMetricChip("↑ Input", "—")
	a.outputChip = newMetricChip("↓ Output", "—")
	a.cacheWChip = newMetricChip("Cache write", "—")
	a.cacheRChip = newMetricChip("Cache read", "—")

	a.projectTable = a.makeTable(
		[]string{"Project", "Messages", "Input", "Output", "Est. Cost"},
		[]float32{220, 90, 90, 90, 100},
		func() int { return len(a.report.Projects) },
		func(row, col int) string {
			if row >= len(a.report.Projects) {
				return ""
			}
			p := a.report.Projects[row]
			switch col {
			case 0:
				return truncate(p.Name, 30)
			case 1:
				return fmt.Sprintf("%d", p.Summary.Messages)
			case 2:
				return formatTokens(p.Summary.InputTokens)
			case 3:
				return formatTokens(p.Summary.OutputTokens)
			case 4:
				return fmt.Sprintf("$%.4f", p.Summary.EstimatedCostUSD)
			}
			return ""
		},
	)
	a.modelTable = a.makeTable(
		[]string{"Model", "Messages", "Input", "Output", "Est. Cost"},
		[]float32{220, 90, 90, 90, 100},
		func() int { return len(a.report.Models) },
		func(row, col int) string {
			if row >= len(a.report.Models) {
				return ""
			}
			m := a.report.Models[row]
			switch col {
			case 0:
				return truncate(m.Name, 30)
			case 1:
				return fmt.Sprintf("%d", m.Summary.Messages)
			case 2:
				return formatTokens(m.Summary.InputTokens)
			case 3:
				return formatTokens(m.Summary.OutputTokens)
			case 4:
				return fmt.Sprintf("$%.4f", m.Summary.EstimatedCostUSD)
			}
			return ""
		},
	)
	a.dailyTable = a.makeTable(
		[]string{"Date", "Messages", "Input", "Output", "Est. Cost"},
		[]float32{180, 90, 90, 90, 100},
		func() int { return len(a.report.Daily) },
		func(row, col int) string {
			if row >= len(a.report.Daily) {
				return ""
			}
			d := a.report.Daily[row]
			switch col {
			case 0:
				return d.Date.Format("Mon, 02 Jan 2006")
			case 1:
				return fmt.Sprintf("%d", d.Summary.Messages)
			case 2:
				return formatTokens(d.Summary.InputTokens)
			case 3:
				return formatTokens(d.Summary.OutputTokens)
			case 4:
				return fmt.Sprintf("$%.4f", d.Summary.EstimatedCostUSD)
			}
			return ""
		},
	)
	a.limitsContainer = container.NewVBox()

	// Period selector
	periodSelect := widget.NewSelect([]string{
		stats.PeriodToday.String(),
		stats.PeriodWeek.String(),
		stats.PeriodMonth.String(),
		stats.PeriodAllTime.String(),
	}, func(s string) {
		switch s {
		case stats.PeriodToday.String():
			a.currentPeriod = stats.PeriodToday
		case stats.PeriodWeek.String():
			a.currentPeriod = stats.PeriodWeek
		case stats.PeriodMonth.String():
			a.currentPeriod = stats.PeriodMonth
		case stats.PeriodAllTime.String():
			a.currentPeriod = stats.PeriodAllTime
		}
		a.refresh()
	})
	periodSelect.SetSelected(stats.PeriodToday.String())

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { a.refresh() })

	toolbar := container.NewHBox(
		newText("Claude Usage", 13, true),
		layout.NewSpacer(),
		a.updatedText,
		periodSelect,
		refreshBtn,
	)

	hero := a.buildHero()
	sep := widget.NewSeparator()

	tabs := container.NewAppTabs(
		container.NewTabItem("Projects", a.projectTable),
		container.NewTabItem("Models", a.modelTable),
		container.NewTabItem("Daily", a.dailyTable),
		container.NewTabItem("Limits", container.NewScroll(a.limitsContainer)),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	header := container.NewVBox(
		container.NewPadded(toolbar),
		container.NewPadded(hero),
		sep,
	)

	return container.NewBorder(header, nil, nil, nil, tabs)
}

// ── Hero section ──────────────────────────────────────────────────────────

func (a *App) buildHero() fyne.CanvasObject {
	// Left: big cost + subtitle
	costLabel := newMutedSmall("ESTIMATED COST")
	subRow := container.NewHBox(a.periodLabel, newMutedSmall(" · "), a.msgsLabel)

	costCol := container.NewVBox(
		costLabel,
		a.costText,
		subRow,
	)

	// Right: 2×2 token chips
	chipGrid := container.NewGridWithColumns(2,
		a.inputChip,
		a.outputChip,
		a.cacheWChip,
		a.cacheRChip,
	)
	tokenCol := container.NewVBox(
		newMutedSmall("TOKEN BREAKDOWN"),
		chipGrid,
	)

	return container.NewGridWithColumns(2,
		container.NewPadded(costCol),
		container.NewPadded(tokenCol),
	)
}

// ── Table ─────────────────────────────────────────────────────────────────

func (a *App) makeTable(
	headers []string,
	colWidths []float32,
	rowCount func() int,
	cellValue func(row, col int) string,
) *widget.Table {
	cols := len(headers)

	t := widget.NewTable(
		func() (int, int) { return rowCount() + 1, cols }, // +1 for header row
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			l := obj.(*widget.Label)
			if id.Row == 0 {
				// Header row
				l.SetText(headers[id.Col])
				l.TextStyle = fyne.TextStyle{Bold: true}
				l.Alignment = fyne.TextAlignLeading
				return
			}
			l.TextStyle = fyne.TextStyle{}
			text := cellValue(id.Row-1, id.Col)
			l.SetText(text)
			// Right-align numeric columns
			if id.Col > 0 {
				l.Alignment = fyne.TextAlignTrailing
			} else {
				l.Alignment = fyne.TextAlignLeading
			}
		},
	)

	for i, w := range colWidths {
		t.SetColumnWidth(i, w)
	}
	t.SetRowHeight(0, 32) // taller header row

	return t
}

// ── Refresh ───────────────────────────────────────────────────────────────

func (a *App) refresh() {
	a.report, a.rolling = a.onRefresh()
	a.cfg = config.Load()
	t := a.report.Total

	a.costText.Text = fmt.Sprintf("$%.4f", t.EstimatedCostUSD)
	a.costText.Refresh()

	a.periodLabel.Text = a.currentPeriod.String()
	a.periodLabel.Refresh()

	a.msgsLabel.Text = fmt.Sprintf("%d messages", t.Messages)
	a.msgsLabel.Refresh()

	a.updatedText.Text = time.Now().Format("15:04:05")
	a.updatedText.Refresh()

	a.inputChip.setValue(formatTokens(t.InputTokens))
	a.outputChip.setValue(formatTokens(t.OutputTokens))
	a.cacheWChip.setValue(formatTokens(t.CacheCreationTokens))
	a.cacheRChip.setValue(formatTokens(t.CacheReadTokens))

	a.projectTable.Refresh()
	a.modelTable.Refresh()
	a.dailyTable.Refresh()
	a.refreshLimitsTab()
}

// ── Limits tab ────────────────────────────────────────────────────────────

func (a *App) refreshLimitsTab() {
	var objects []fyne.CanvasObject

	// Rolling windows section
	objects = append(objects, sectionHeader("ROLLING WINDOWS"))
	for _, r := range a.rolling {
		objects = append(objects, a.makeRollingCard(r))
	}

	// Budget gauges section
	hasGauges := a.cfg.DailyBudgetUSD > 0 || a.cfg.MonthlyBudgetUSD > 0 ||
		(a.cfg.HourlyTokenLimit > 0 && len(a.rolling) > 0)

	if hasGauges {
		objects = append(objects, sectionHeader("BUDGET GAUGES"))

		if a.cfg.DailyBudgetUSD > 0 {
			todayReport := stats.Compute(a.report.AllEntries, stats.PeriodToday)
			objects = append(objects, a.makeBudgetCard(
				"Daily Budget", todayReport.Total.EstimatedCostUSD,
				a.cfg.DailyBudgetUSD, "$", stats.DayBoundaryReset(),
			))
		}
		if a.cfg.MonthlyBudgetUSD > 0 {
			monthReport := stats.Compute(a.report.AllEntries, stats.PeriodMonth)
			objects = append(objects, a.makeBudgetCard(
				"Monthly Budget", monthReport.Total.EstimatedCostUSD,
				a.cfg.MonthlyBudgetUSD, "$", stats.MonthBoundaryReset(),
			))
		}
		if a.cfg.HourlyTokenLimit > 0 && len(a.rolling) > 0 {
			objects = append(objects, a.makeBudgetCard(
				"Hourly Tokens", float64(a.rolling[0].TotalTokens),
				float64(a.cfg.HourlyTokenLimit), "",
				time.Now().Truncate(time.Hour).Add(time.Hour),
			))
		}
	}

	objects = append(objects,
		sectionHeader("CONFIGURE LIMITS"),
		a.makeSettingsForm(),
	)

	a.limitsContainer.Objects = objects
	a.limitsContainer.Refresh()
}

func (a *App) makeRollingCard(r stats.RollingUsage) fyne.CanvasObject {
	windowLabel := newText(fmt.Sprintf("Last %s", formatDuration(r.Window)), 13, true)
	costVal := newText(fmt.Sprintf("$%.4f", r.CostUSD), 13, false)
	costVal.Color = colorAccent

	row := container.NewHBox(
		windowLabel,
		layout.NewSpacer(),
		newMutedSmall(fmt.Sprintf("%s tokens", formatTokens(r.TotalTokens))),
		newMutedSmall("  ·  "),
		costVal,
		newMutedSmall("  ·  "),
		newMutedSmall(fmt.Sprintf("%d msgs", r.Messages)),
	)
	return container.NewPadded(row)
}

func (a *App) makeBudgetCard(title string, used, limit float64, prefix string, reset time.Time) fyne.CanvasObject {
	pct := used / limit
	if pct > 1 {
		pct = 1
	}

	bar := widget.NewProgressBar()
	bar.Min, bar.Max = 0, 1
	bar.SetValue(pct)
	// Color the bar based on usage level
	_ = pct // Fyne ProgressBar doesn't support custom colors natively; use label cues instead

	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	untilReset := time.Until(reset)

	var usedStr, remStr string
	if prefix == "$" {
		usedStr = fmt.Sprintf("$%.4f used", used)
		remStr = fmt.Sprintf("$%.4f left", remaining)
	} else {
		usedStr = fmt.Sprintf("%s used", formatTokens(int64(used)))
		remStr = fmt.Sprintf("%s left", formatTokens(int64(remaining)))
	}

	titleText := newText(title, 12, true)
	pctText := newText(fmt.Sprintf("%.1f%%", pct*100), 12, false)
	switch {
	case pct >= 0.9:
		pctText.Color = colorDanger
	case pct >= 0.7:
		pctText.Color = colorWarn
	default:
		pctText.Color = colorSuccess
	}

	topRow := container.NewHBox(titleText, layout.NewSpacer(), pctText)
	bottomRow := container.NewHBox(
		newMutedSmall(usedStr),
		layout.NewSpacer(),
		newMutedSmall(remStr),
		layout.NewSpacer(),
		newMutedSmall("resets in "+formatDuration(untilReset.Round(time.Minute))),
	)

	return container.NewPadded(container.NewVBox(topRow, bar, bottomRow))
}

func (a *App) makeSettingsForm() fyne.CanvasObject {
	dailyEntry := widget.NewEntry()
	dailyEntry.SetText(fmt.Sprintf("%.2f", a.cfg.DailyBudgetUSD))
	dailyEntry.SetPlaceHolder("e.g. 10.00")

	monthlyEntry := widget.NewEntry()
	monthlyEntry.SetText(fmt.Sprintf("%.2f", a.cfg.MonthlyBudgetUSD))
	monthlyEntry.SetPlaceHolder("0 = off")

	hourlyEntry := widget.NewEntry()
	hourlyEntry.SetText(fmt.Sprintf("%d", a.cfg.HourlyTokenLimit))
	hourlyEntry.SetPlaceHolder("0 = off")

	status := widget.NewLabel("")
	status.TextStyle = fyne.TextStyle{Italic: true}

	saveBtn := widget.NewButton("Save changes", func() {
		daily, err1 := strconv.ParseFloat(dailyEntry.Text, 64)
		monthly, err2 := strconv.ParseFloat(monthlyEntry.Text, 64)
		hourly, err3 := strconv.ParseInt(hourlyEntry.Text, 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			status.SetText("Invalid values — numbers only")
			return
		}
		cfg := config.Config{
			DailyBudgetUSD:   daily,
			MonthlyBudgetUSD: monthly,
			HourlyTokenLimit: hourly,
		}
		if err := config.Save(cfg); err != nil {
			status.SetText("Save failed: " + err.Error())
			return
		}
		a.cfg = cfg
		status.SetText("Saved!")
		a.refreshLimitsTab()
	})

	form := widget.NewForm(
		widget.NewFormItem("Daily budget ($)", dailyEntry),
		widget.NewFormItem("Monthly budget ($)", monthlyEntry),
		widget.NewFormItem("Hourly token limit", hourlyEntry),
	)

	return container.NewPadded(container.NewVBox(form, container.NewHBox(saveBtn, status)))
}

// ── metricChip ────────────────────────────────────────────────────────────

// metricChip is a small label+value pair used in the hero token breakdown.
type metricChip struct {
	widget.BaseWidget
	label *canvas.Text
	value *canvas.Text
}

func newMetricChip(label, value string) *metricChip {
	c := &metricChip{
		label: newMutedSmall(label),
		value: newText(value, 13, true),
	}
	c.ExtendBaseWidget(c)
	return c
}

func (c *metricChip) setValue(v string) {
	c.value.Text = v
	c.value.Refresh()
}

func (c *metricChip) CreateRenderer() fyne.WidgetRenderer {
	col := container.NewVBox(c.label, c.value)
	return widget.NewSimpleRenderer(col)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func sectionHeader(text string) fyne.CanvasObject {
	t := newMutedSmall(text)
	return container.NewVBox(
		widget.NewSeparator(),
		container.NewPadded(t),
	)
}

func newText(text string, size float32, bold bool) *canvas.Text {
	t := canvas.NewText(text, theme.Color(theme.ColorNameForeground))
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: bold}
	return t
}

func newMutedSmall(text string) *canvas.Text {
	t := canvas.NewText(text, colorMuted)
	t.TextSize = 11
	return t
}

func color32(r, g, b, a uint8) color32val {
	return color32val{r, g, b, a}
}

type color32val struct{ R, G, B, A uint8 }

func (c color32val) RGBA() (uint32, uint32, uint32, uint32) {
	r := uint32(c.R)
	g := uint32(c.G)
	b := uint32(c.B)
	a := uint32(c.A)
	r = r | (r << 8)
	g = g | (g << 8)
	b = b | (b << 8)
	a = a | (a << 8)
	return r, g, b, a
}

func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func formatDuration(d time.Duration) string {
	if d >= 24*time.Hour {
		days := int(d.Hours() / 24)
		hours := int(d.Hours()) % 24
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if d >= time.Hour {
		mins := int(d.Minutes()) % 60
		if mins == 0 {
			return fmt.Sprintf("%dh", int(d.Hours()))
		}
		return fmt.Sprintf("%dh %dm", int(d.Hours()), mins)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
