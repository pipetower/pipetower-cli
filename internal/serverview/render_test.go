package serverview

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/pipetower/pipetower-cli/internal/api"
)

func ptr[T any](v T) *T { return &v }

func TestDashboardShowsMetrics(t *testing.T) {
	now := time.Now()
	server := api.Server{
		Name: "pipetower.com", Host: "5.75.144.227", Port: 22, Connected: true,
		Metrics: &api.Metrics{
			Hostname: "prod", OperatingSystem: "Ubuntu 22.04.5 LTS", Kernel: "6.8.0", Architecture: "x86_64",
			CPU:        api.CPU{Model: "Intel Xeon Processor (Skylake, IBRS)", Cores: ptr(4), UsagePercent: ptr(49.0)},
			Load:       api.Load{Avg1: ptr(0.61), Avg5: ptr(0.69), Avg15: ptr(0.67)},
			Memory:     api.Usage{TotalBytes: ptr(7.6 * 1024 * 1024 * 1024), UsedBytes: ptr(3.3 * 1024 * 1024 * 1024), UsedPercent: ptr(44.0)},
			Disk:       api.Usage{TotalBytes: ptr(75.0 * 1024 * 1024 * 1024), UsedBytes: ptr(26.0 * 1024 * 1024 * 1024), UsedPercent: ptr(35.0)},
			MeasuredAt: ptr(now.Add(-2 * time.Minute)),
		},
	}
	runs := []api.Run{{ID: "35e1d2", Status: api.StatusSuccessful, Executable: &api.Reference{Type: "Workflow", Name: "Deploy Apdf"}, TriggeredBy: &api.Person{Name: "Jamil"}}}

	out := ansi.Strip(Dashboard(server, runs, now))

	for _, want := range []string{"pipetower.com", "● Connected", "5.75.144.227:22", "Ubuntu 22.04.5 LTS", "49%", "4 × Intel Xeon", "3.3 GB of 7.6 GB", "26 GB of 75 GB", "0.61  0.69  0.67", "Measured 2 minutes ago", "Workflow Deploy Apdf", "#35e1d2"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestDashboardOfDisconnectedServer(t *testing.T) {
	out := ansi.Strip(Dashboard(api.Server{Name: "db 1", Host: "10.0.0.1", Port: 22}, nil, time.Now()))

	for _, want := range []string{"○ Disconnected", `pt server connect "db 1"`, "No runs yet"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
