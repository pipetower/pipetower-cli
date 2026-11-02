// Package serverview renders servers: the list with mini usage bars and the
// dashboard of one server with its metrics and latest runs.
package serverview

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/present"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

const (
	// barWidth is the widest a usage bar gets, it shrinks in narrow terminals before the details do.
	barWidth = 20
	// minBarWidth is the narrowest usage bar.
	minBarWidth = 8
	// detailRoom is the room the details of a usage bar get before the bar shrinks, e.g. "3.3 GB of 7.6 GB".
	detailRoom   = 18
	miniBarWidth = 6
	// cardWidth is the widest the card gets, narrower terminals get a narrower card.
	cardWidth = 78
	// cardFrame is the width of the border and padding of the card.
	cardFrame = 6
	// labelWidth is the width of the labels in the card, e.g. "Hostname".
	labelWidth = 10
	// maxHostValue limits the values of the left host column, e.g. a very long kernel version.
	maxHostValue = 40
)

var card = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ui.ColorFaint).
	Padding(0, 2)

// Dashboard renders a server with its metrics and latest runs, at most as wide as the width.
func Dashboard(server api.Server, runs []api.Run, now time.Time, width int) string {
	outer := min(cardWidth, width-1)
	inner := outer - cardFrame

	sections := []string{heading(server, inner)}
	if server.Metrics == nil {
		sections = append(sections, noMetrics(server))
	} else {
		sections = append(sections, hostInfo(server.Metrics, inner), usage(server.Metrics, inner), measured(server.Metrics, now))
	}
	content := fitLines(strings.Join(sections, "\n\n"), inner)

	return ui.Indent(card.Width(outer).Render(content), " ") + "\n" + latestRuns(server.ID, runs, now, width)
}

// heading renders the name, address, tags and connection state.
func heading(server api.Server, inner int) string {
	title := ui.Title.Render(server.Name)
	gap := inner - lipgloss.Width(title) - lipgloss.Width(ui.Connection(server.Connected))
	first := title + strings.Repeat(" ", max(2, gap)) + ui.Connection(server.Connected)

	address := ui.Muted.Render(fmt.Sprintf("%s:%d", server.Host, server.Port))
	for _, tag := range server.Tags {
		address += "  " + ui.Tag(tag.Name, tag.Color)
	}
	return first + "\n" + address
}

// hostInfo renders hostname, operating system, kernel and architecture in two columns. The left
// column grows with its longest value, so that a long kernel version never runs into the right one.
func hostInfo(m *api.Metrics, inner int) string {
	hostname, kernel := truncate(ui.Or(m.Hostname, "–"), maxHostValue), truncate(ui.Or(m.Kernel, "–"), maxHostValue)
	left := lipgloss.NewStyle().Width(max(34, labelWidth+max(lipgloss.Width(hostname), lipgloss.Width(kernel))+2))
	rightWidth := max(1, inner-left.GetWidth()-8)
	row := func(k1, v1, k2, v2 string) string {
		return left.Render(ui.Muted.Render(pad(k1, labelWidth))+v1) + ui.Muted.Render(pad(k2, 8)) + truncate(ui.Or(v2, "–"), rightWidth)
	}
	return row("Hostname", hostname, "OS", m.OperatingSystem) + "\n" + row("Kernel", kernel, "Arch", m.Architecture)
}

// usage renders CPU, memory and disk bars and the load average.
func usage(m *api.Metrics, inner int) string {
	bar := max(minBarWidth, min(barWidth, inner-labelWidth-2-4-3-detailRoom))
	return strings.Join([]string{
		meter("CPU", m.CPU.UsagePercent, cpuText(m.CPU), bar),
		meter("Memory", m.Memory.UsedPercent, usedOf(m.Memory), bar),
		meter("Disk", m.Disk.UsedPercent, usedOf(m.Disk), bar),
		ui.Muted.Render(pad("Load", labelWidth)) + load(m.Load, m.CPU.Cores) + ui.Faint.Render("    1m · 5m · 15m"),
	}, "\n")
}

// meter renders one labeled usage bar with its percentage and details.
func meter(label string, percent *float64, detail string, bar int) string {
	value := "  –  "
	if percent != nil {
		value = ui.Fg(ui.UsageColor(*percent)).Bold(true).Render(fmt.Sprintf("%4s", ui.Percent(*percent)))
	}
	return ui.Muted.Render(pad(label, labelWidth)) + ui.Bar(percent, bar) + "  " + value + "   " + ui.Muted.Render(detail)
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
			values = append(values, ui.Faint.Render("–    "))
			continue
		}
		values = append(values, ui.Fg(ui.LoadColor(*value, cores)).Render(fmt.Sprintf("%-5.2f", *value)))
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

// latestRuns renders the latest runs of the server below the card, each with its status on this
// server like in the web app: a run on several servers may fail on one and succeed on another.
func latestRuns(serverID string, runs []api.Run, now time.Time, width int) string {
	heading := "\n " + ui.Bold.Render("Latest runs") + "\n"
	if len(runs) == 0 {
		return heading + ui.Muted.Render("  No runs yet")
	}

	rows := make([][]string, 0, len(runs))
	for _, run := range runs {
		run.Status = statusOn(run, serverID)
		rows = append(rows, present.RunRow(run, now))
	}
	return heading + ui.Indent(ui.Table(present.RunHeaders, rows, width-2), "  ")
}

// ListRow renders a server as a row of the server list.
func ListRow(server api.Server) []string {
	row := []string{ui.ConnectionMark(server.Connected) + " " + ui.Bold.Render(server.Name), ui.Muted.Render(server.Host)}
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

// fitLines cuts every line of the text at the width, with "…" where something was cut.
func fitLines(text string, width int) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}

// statusOn returns the status of the run's execution on the server, or the run's status without one.
func statusOn(run api.Run, serverID string) string {
	for _, execution := range run.Executions {
		if execution.Server.ID == serverID {
			return execution.Status
		}
	}
	return run.Status
}
