package duration_test

import (
	"testing"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/eden9th/bedrock/duration"
)

func TestUnmarshalText_Valid(t *testing.T) {
	cases := []struct {
		input    string
		expected time.Duration
	}{
		{"1s", time.Second},
		{"30s", 30 * time.Second},
		{"1m", time.Minute},
		{"1h30m", 90 * time.Minute},
		{"500ms", 500 * time.Millisecond},
	}

	for _, tc := range cases {
		var d duration.Duration
		if err := d.UnmarshalText([]byte(tc.input)); err != nil {
			t.Errorf("UnmarshalText(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if d.Duration() != tc.expected {
			t.Errorf("UnmarshalText(%q): expected %v, got %v", tc.input, tc.expected, d.Duration())
		}
	}
}

func TestUnmarshalText_Invalid(t *testing.T) {
	var d duration.Duration
	err := d.UnmarshalText([]byte("not-a-duration"))
	if err == nil {
		t.Fatal("expected error for invalid duration string, got nil")
	}
}

func TestDuration_ReturnsTimeDuration(t *testing.T) {
	var d duration.Duration
	if err := d.UnmarshalText([]byte("2h")); err != nil {
		t.Fatal(err)
	}
	if d.Duration() != 2*time.Hour {
		t.Fatalf("expected 2h, got %v", d.Duration())
	}
}

func TestString(t *testing.T) {
	var d duration.Duration
	if err := d.UnmarshalText([]byte("1h30m")); err != nil {
		t.Fatal(err)
	}
	s := d.String()
	if s == "" {
		t.Fatal("String() returned empty string")
	}
	// time.Duration.String() 对 1h30m 输出 "1h30m0s"
	parsed, err := time.ParseDuration(s)
	if err != nil {
		t.Fatalf("String() returned unparseable value %q: %v", s, err)
	}
	if parsed != 90*time.Minute {
		t.Fatalf("expected 1h30m, got %v", parsed)
	}
}

func TestTOMLRoundTrip(t *testing.T) {
	type Config struct {
		Timeout duration.Duration `toml:"timeout"`
		Retry   duration.Duration `toml:"retry"`
	}

	input := `
timeout = "30s"
retry = "1m"
`
	var cfg Config
	if _, err := toml.Decode(input, &cfg); err != nil {
		t.Fatalf("TOML decode failed: %v", err)
	}
	if cfg.Timeout.Duration() != 30*time.Second {
		t.Errorf("expected timeout=30s, got %v", cfg.Timeout.Duration())
	}
	if cfg.Retry.Duration() != time.Minute {
		t.Errorf("expected retry=1m, got %v", cfg.Retry.Duration())
	}
}
