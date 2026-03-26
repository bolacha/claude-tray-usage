package main

import (
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/systray"

	"claude_tray_usage/parser"
	"claude_tray_usage/stats"
	uipkg "claude_tray_usage/ui"
)

func main() {
	a := app.New()
	a.SetIcon(claudeIcon())

	desk, ok := a.(desktop.App)
	if !ok {
		log.Fatal("system tray not supported on this platform/driver")
	}

	var entries []parser.UsageEntry
	loadEntries := func() {
		var err error
		entries, err = parser.ParseAll()
		if err != nil {
			log.Printf("parse error: %v", err)
		}
	}
	loadEntries()

	rollingWindows := []time.Duration{time.Hour, 5 * time.Minute}

	ui := uipkg.New(a, func() (stats.Report, []stats.RollingUsage) {
		loadEntries()
		report := stats.Compute(entries, stats.PeriodToday)
		var rolling []stats.RollingUsage
		for _, w := range rollingWindows {
			rolling = append(rolling, stats.ComputeRolling(entries, w))
		}
		return report, rolling
	})

	// System tray menu
	openItem := fyne.NewMenuItem("Open Usage Tracker", func() { ui.ShowWindow() })
	quitItem := fyne.NewMenuItem("Quit", func() { a.Quit() })
	trayMenu := fyne.NewMenu("Claude Usage", openItem, fyne.NewMenuItemSeparator(), quitItem)
	desk.SetSystemTrayMenu(trayMenu)
	desk.SetSystemTrayIcon(claudeIcon())

	// Update tray title once systray has initialised (Fyne calls RunWithExternalLoop
	// during SetSystemTrayMenu, so a brief yield is enough).
	go func() {
		time.Sleep(2 * time.Second)
		updateTrayTitle(entries)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ui.Refresh()
			updateTrayTitle(entries)
		}
	}()

	ui.ShowWindow()
	a.Run()
}

// updateTrayTitle sets the menu-bar text next to the icon.
// Format: ↑134.0K ↓1.94M  (input ↑, output ↓, today's totals)
func updateTrayTitle(entries []parser.UsageEntry) {
	report := stats.Compute(entries, stats.PeriodToday)
	t := report.Total
	title := fmt.Sprintf("↑%s ↓%s", formatTokens(t.InputTokens+t.CacheCreationTokens), formatTokens(t.OutputTokens))
	systray.SetTitle(title)
	systray.SetTooltip(fmt.Sprintf("Claude today: $%.4f | %d messages", t.EstimatedCostUSD, t.Messages))
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

// claudeIcon returns an SVG of the Anthropic "A" symbol on an orange background.
// Outer A: apex→bottom-right→inner-right→inner-apex→inner-left→bottom-left
// Inner triangle (evenodd cutout) creates the hollow centre of the A.
func claudeIcon() fyne.Resource {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="5" fill="#CC6B2C"/>
  <path fill="white" fill-rule="evenodd"
    d="M16 5 L27 27 L22 27 L16 15 L10 27 L5 27 Z
       M16 17 L11 25 L21 25 Z"/>
</svg>`)
	return fyne.NewStaticResource("claude.svg", svg)
}
