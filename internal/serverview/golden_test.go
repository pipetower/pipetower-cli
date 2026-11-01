package serverview

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// The golden tests pin the complete rendered views. After an intended change of a view,
// review the diff and update the files with: go test ./internal/serverview -update

var goldenNow = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

const gigabyte = 1024 * 1024 * 1024

// connectedServer is a server with all metrics, like the dashboard in the web.
func connectedServer() api.Server {
	return api.Server{
		Name: "pipetower.com", Host: "5.75.144.227", Port: 22, Connected: true, Tags: []api.Tag{{Name: "prod"}},
		Metrics: &api.Metrics{
			Hostname: "prod", OperatingSystem: "Ubuntu 22.04.5 LTS", Kernel: "Linux 5.15.0-176-generic", Architecture: "x86-64",
			CPU:        api.CPU{Model: "Intel Xeon Processor (Skylake, IBRS)", Cores: ptr(4), UsagePercent: ptr(49.0)},
			Load:       api.Load{Avg1: ptr(0.61), Avg5: ptr(0.69), Avg15: ptr(0.67)},
			Memory:     api.Usage{TotalBytes: ptr(7.6 * gigabyte), UsedBytes: ptr(3.3 * gigabyte), UsedPercent: ptr(44.0)},
			Disk:       api.Usage{TotalBytes: ptr(75.0 * gigabyte), UsedBytes: ptr(26.0 * gigabyte), UsedPercent: ptr(35.0)},
			MeasuredAt: ptr(goldenNow.Add(-2 * time.Minute)),
		},
	}
}

// dashboardRuns are the runs of the dashboard: a finished and a running one.
func dashboardRuns() []api.Run {
	started := goldenNow.Add(-48 * time.Hour)
	return []api.Run{
		{ID: "4k1x9z", Status: api.StatusRunning, StartedAt: ptr(goldenNow.Add(-20 * time.Second)), Executable: &api.Reference{Type: "Action", Name: "Restart PHP"}, TriggeredBy: &api.Person{Name: "Jamil Soufan"}},
		{ID: "35e1d2", Status: api.StatusSuccessful, StartedAt: &started, FinishedAt: ptr(started.Add(126 * time.Second)), Executable: &api.Reference{Type: "Workflow", Name: "Deploy Apdf"}, TriggeredBy: &api.Person{Name: "Jamil Soufan"}},
	}
}

func TestGoldenDashboard(t *testing.T) {
	golden.RequireEqual(t, ansi.Strip(Dashboard(connectedServer(), dashboardRuns(), goldenNow, 100)))
}

func TestGoldenDashboardOfDisconnectedServer(t *testing.T) {
	golden.RequireEqual(t, ansi.Strip(Dashboard(api.Server{Name: "db 1", Host: "10.0.0.1", Port: 2222}, nil, goldenNow, 100)))
}

func TestGoldenServerList(t *testing.T) {
	disconnected := api.Server{Name: "db 1", Host: "10.0.0.1", Port: 22}
	rows := [][]string{ListRow(connectedServer()), ListRow(disconnected)}

	golden.RequireEqual(t, ansi.Strip(ui.Table(ListHeaders, rows, 0)))
}

func TestGoldenDashboardInANarrowTerminal(t *testing.T) {
	golden.RequireEqual(t, ansi.Strip(Dashboard(connectedServer(), dashboardRuns(), goldenNow, 60)))
}
