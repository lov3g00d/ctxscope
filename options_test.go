package ctxscope

import (
	"strings"
	"testing"
	"time"
)

func TestNewConfigUsesDefaults(t *testing.T) {
	cfg, err := newConfig()

	if err != nil {
		t.Fatalf("create default config: %v", err)
	}

	if cfg.grace != defaultGrace {
		t.Errorf("got grace %s, want %s", cfg.grace, defaultGrace)
	}

	if cfg.pollInterval != defaultPollInterval {
		t.Errorf(
			"got poll interval %s, want %s",
			cfg.pollInterval,
			defaultPollInterval,
		)
	}

	if cfg.maxPollInterval != defaultMaxPollInterval {
		t.Errorf(
			"got maximum poll interval %s, want %s",
			cfg.maxPollInterval,
			defaultMaxPollInterval,
		)
	}

	if cfg.startupTimeout != 0 {
		t.Errorf("got startup timeout %s, want 0", cfg.startupTimeout)
	}
}

func TestNewConfigAppliesOptions(t *testing.T) {
	cfg, err := newConfig(
		WithName("worker shutdown"),
		WithGrace(time.Second),
		WithPollInterval(10*time.Millisecond),
		WithMaxPollInterval(80*time.Millisecond),
		WithStartupTimeout(500*time.Millisecond),
	)

	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	if cfg.name != "worker shutdown" {
		t.Errorf("got name %q, want %q", cfg.name, "worker shutdown")
	}

	if cfg.grace != time.Second {
		t.Errorf("got grace %s, want %s", cfg.grace, time.Second)
	}

	if cfg.pollInterval != 10*time.Millisecond {
		t.Errorf(
			"got poll interval %s, want %s",
			cfg.pollInterval,
			10*time.Millisecond,
		)
	}

	if cfg.maxPollInterval != 80*time.Millisecond {
		t.Errorf(
			"got maximum poll interval %s, want %s",
			cfg.maxPollInterval,
			80*time.Millisecond,
		)
	}

	if cfg.startupTimeout != 500*time.Millisecond {
		t.Errorf(
			"got startup timeout %s, want %s",
			cfg.startupTimeout,
			500*time.Millisecond,
		)
	}
}

func TestNewConfigRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name        string
		options     []Option
		wantMessage string
	}{
		{
			name:        "nil option",
			options:     []Option{nil},
			wantMessage: "nil option",
		},
		{
			name:        "zero grace",
			options:     []Option{WithGrace(0)},
			wantMessage: "grace period",
		},
		{
			name:        "negative grace",
			options:     []Option{WithGrace(-time.Second)},
			wantMessage: "grace period",
		},
		{
			name:        "zero poll interval",
			options:     []Option{WithPollInterval(0)},
			wantMessage: "poll interval",
		},
		{
			name:        "zero maximum poll interval",
			options:     []Option{WithMaxPollInterval(0)},
			wantMessage: "maximum poll interval",
		},
		{
			name:        "negative startup timeout",
			options:     []Option{WithStartupTimeout(-time.Second)},
			wantMessage: "startup timeout",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newConfig(test.options...)

			if err == nil {
				t.Fatal("expected an error")
			}

			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("error %q does not contain %q", err, test.wantMessage)
			}
		})
	}
}
