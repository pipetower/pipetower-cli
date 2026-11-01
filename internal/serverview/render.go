// Package serverview renders servers: the list with mini usage bars and the
// dashboard of one server with its metrics and latest runs.
package serverview

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/present"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

const (
	barWidth     = 20
	miniBarWidth = 8
	cardWidth    = 78
	// innerWidth is the content width of the card: without border and padding.
	innerWidth = cardWidth - 6
)

var card = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ui.ColorFaint).
	Padding(0, 2).
	Width(cardWidth)

// Dashboard renders a server with its metrics and latest runs.
func Dashboard(server api.Server, runs []api.Run, now time.Time) string {
	sections := []string{heading(server)}
	if server.Metrics == nil {
		sections = append(sections, noMetrics(server))
	} else {
		sections = append(sections, hostInfo(server.Metrics), usage(server.Metrics), measured(server.Metrics, now))
	}

	return card.Render(strings.Join(sections, "\n\n")) + "\n" + latestRuns(runs, now)
}

// heading renders the name, address, tags and connection state.
func heading(server api.Server) string {
	title := ui.Title.Render(server.Name)
	gap := innerWidth - lipgloss.Width(title) - lipgloss.Width(ui.Connection(server.Connected))
	first := title + strings.Repeat(" ", max(2, gap)) + ui.Connection(server.Connected)

	address := ui.Muted.Render(fmt.Sprintf("%s:%d", server.Host, server.Port))
	for _, tag := range server.Tags {
		address += "  " + ui.Tag(tag.Name, tag.Color)
	}
	return first + "\n" + address
}

// hostInfo renders hostname, operating system, kernel and architecture.
func hostInfo(m *api.Metrics) string {
	left := lipgloss.NewStyle().Width(34)
	row := func(k1, v1, k2, v2 string) string {
		return left.Render(ui.Muted.Render(pad(k1, 10))+ui.Or(v1, "–")) + ui.Muted.Render(pad(k2, 8)) + ui.Or(v2, "–")
	}
	return row("Hostname", m.Hostname, "OS", m.OperatingSystem) + "\n" + row("Kernel", m.Kernel, "Arch", m.Architecture)
}

// usage renders CPU, memory and disk bars and the load average.
func usage(m *api.Metrics) string {
	return strings.Join([]string{
		meter("CPU", m.CPU.UsagePercent, cpuText(m.CPU)),
		meter("Memory", m.Memory.UsedPercent, usedOf(m.Memory)),
		meter("Disk", m.Disk.UsedPercent, usedOf(m.Disk)),
		ui.Muted.Render(pad("Load", 10)) + load(m.Load, m.CPU.Cores) + ui.Faint.Render("    1m · 5m · 15m"),
	}, "\n")
}

// meter renders one labeled usage bar with its percentage and details.
func meter(label string, percent *float64, detail string) string {
	value := "  –  "
	if percent != nil {
		value = ui.Fg(ui.UsageColor(*percent)).Bold(true).Render(fmt.Sprintf("%4s", ui.Percent(*percent)))
	}
	return ui.Muted.Render(pad(label, 10)) + ui.Bar(percent, barWidth) + "  " + value + "   " + ui.Muted.Render(detail)
}

// cpuText renders "4 × Intel Xeon Processor".
func cpuText(cpu api.CPU) string {
	model := truncate(cpu.Model, 28)
	if cpu.Cores != nil {
		return fmt.Sprintf("%d × %s", *cpu.Cores, ui.Or(model, "CPU"))
	}
	return model
}

// usedOf renders "3.3 GB of 7.6 GB".
func usedOf(u api.Usage) string {
	if u.UsedBytes == nil || u.TotalBytes == nil {
		return ""
	}
	return ui.Bytes(*u.UsedBytes) + " of " + ui.Bytes(*u.TotalBytes)
}

// load renders the three load averages colored relative to the cores.
func load(l api.Load, cores *int) string {
	values := make([]string, 0, 3)
	for _, value := range []*float64{l.Avg1, l.Avg5, l.Avg15} {
		if value == nil {
			values = append(values, ui.Faint.Render("  –  "))
			continue
		}
		values = append(values, ui.Fg(ui.LoadColor(*value, cores)).Render(fmt.Sprintf("%5.2f", *value)))
	}
	return strings.Join(values, " ")
}

// measured tells how old the metrics are.
func measured(m *api.Metrics, now time.Time) string {
	if m.MeasuredAt == nil {
		return ui.Faint.Render("No measurement yet")
	}
	return ui.Faint.Render("Measured " + ui.Ago(m.MeasuredAt, now))
}

// noMetrics explains why a server has no metrics.
func noMetrics(server api.Server) string {
	if !server.Connected {
		return ui.Muted.Render("Not connected, so there are no metrics.") + "\n" +
			ui.Muted.Render("Connect it with ") + ui.Code.Render("pt server connect "+ui.Arg(server.Name))
	}
	return ui.Muted.Render("No metrics measured yet, they appear within a minute.")
}

// latestRuns renders the latest runs of the server below the card.
func latestRuns(runs []api.Run, now time.Time) string {
	heading := "\n" + ui.Bold.Render("  Latest runs") + "\n"
	if len(runs) == 0 {
		return heading + ui.Muted.Render("  No runs yet") + "\n"
	}

	rows := make([][]string, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, present.RunRow(run, now))
	}
	return heading + indent(ui.Table(present.RunHeaders, rows))
}

// ListRow renders a server as a row of the server list.
func ListRow(server api.Server) []string {
	status := ui.Success.Render("●")
	if !server.Connected {
		status = ui.Faint.Render("○")
	}

	row := []string{status + " " + ui.Bold.Render(server.Name), ui.Muted.Render(server.Host)}
	if m := server.Metrics; m != nil {
		return append(row, mini(m.CPU.UsagePercent), mini(m.Memory.UsedPercent), mini(m.Disk.UsedPercent), load1(m))
	}
	return append(row, ui.Faint.Render("–"), ui.Faint.Render("–"), ui.Faint.Render("–"), ui.Faint.Render("–"))
}

// ListHeaders are the columns of the server list.
var ListHeaders = []string{"SERVER", "HOST", "CPU", "MEMORY", "DISK", "LOAD"}

// mini renders a small usage bar with its percentage.
func mini(percent *float64) string {
	if percent == nil {
		return ui.Faint.Render("–")
	}
	return ui.Bar(percent, miniBarWidth) + " " + ui.Fg(ui.UsageColor(*percent)).Render(fmt.Sprintf("%4s", ui.Percent(*percent)))
}

// load1 renders the one minute load average.
func load1(m *api.Metrics) string {
	if m.Load.Avg1 == nil {
		return ui.Faint.Render("–")
	}
	return ui.Fg(ui.LoadColor(*m.Load.Avg1, m.CPU.Cores)).Render(fmt.Sprintf("%.2f", *m.Load.Avg1))
}

// pad pads a label to a width.
func pad(text string, width int) string {
	return fmt.Sprintf("%-*s", width, text)
}

// truncate shortens text to a number of characters.
func truncate(text string, length int) string {
	runes := []rune(text)
	if len(runes) <= length {
		return text
	}
	return string(runes[:length-1]) + "…"
}

// indent indents every line by two spaces.
func indent(text string) string {
	return "  " + strings.ReplaceAll(text, "\n", "\n  ")
}
