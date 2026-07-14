package monitor_test

import (
	"testing"

	"github.com/eden9th/bedrock/monitor"
)

func TestCounterIncWithDefaultLabels(t *testing.T) {
	c := monitor.NewCounter("requests_total", "requests", []string{"service", "method"})
	c.DefaultLabels = map[string]string{"service": "api"}

	c.Inc("GET")

	if got := c.Value("GET"); got != 1 {
		t.Fatalf("expected value=1, got %d", got)
	}
}

func TestGaugeSetWithOnlyDefaultLabels(t *testing.T) {
	g := monitor.NewGauge("db_pool_open_connections", "open connections", []string{"name"})
	g.DefaultLabels = map[string]string{"name": "postgres"}

	g.Set(3)

	if got := g.Value(); got != 3 {
		t.Fatalf("expected value=3, got %v", got)
	}
}
