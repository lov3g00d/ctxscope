package ctxscope

import (
	"errors"
	"fmt"
	"time"
)

const (
	defaultGrace           = 250 * time.Millisecond
	defaultPollInterval    = 5 * time.Millisecond
	defaultMaxPollInterval = 40 * time.Millisecond
)

// Option configures an inspection.
type Option func(*config)

type config struct {
	name            string
	grace           time.Duration
	pollInterval    time.Duration
	maxPollInterval time.Duration
	startupTimeout  time.Duration
}

// WithName assigns a human-readable operation name to the report.
func WithName(name string) Option {
	return func(cfg *config) {
		cfg.name = name
	}
}

// WithGrace sets how long Inspect waits for labeled goroutines to stop after
// cancellation. The duration must be greater than zero.
func WithGrace(grace time.Duration) Option {
	return func(cfg *config) {
		cfg.grace = grace
	}
}

// WithPollInterval sets the initial delay between goroutine profile captures.
// Polling backs off adaptively up to the maximum interval. The duration must be
// greater than zero.
func WithPollInterval(interval time.Duration) Option {
	return func(cfg *config) {
		cfg.pollInterval = interval
	}
}

// WithMaxPollInterval caps the adaptive delay between profile captures. The
// duration must be greater than zero.
func WithMaxPollInterval(interval time.Duration) Option {
	return func(cfg *config) {
		cfg.maxPollInterval = interval
	}
}

// WithStartupTimeout limits how long the start function may run before
// cancellation begins. A zero duration leaves startup unbounded.
func WithStartupTimeout(timeout time.Duration) Option {
	return func(cfg *config) {
		cfg.startupTimeout = timeout
	}
}

func newConfig(options ...Option) (config, error) {
	cfg := config{
		grace:           defaultGrace,
		pollInterval:    defaultPollInterval,
		maxPollInterval: defaultMaxPollInterval,
	}

	for _, option := range options {
		if option == nil {
			return config{}, errors.New("ctxscope: nil option")
		}

		option(&cfg)
	}

	if cfg.grace <= 0 {
		return config{}, fmt.Errorf(
			"ctxscope: grace period must be greater than zero: %s",
			cfg.grace,
		)
	}

	if cfg.pollInterval <= 0 {
		return config{}, fmt.Errorf(
			"ctxscope: poll interval must be greater than zero: %s",
			cfg.pollInterval,
		)
	}

	if cfg.maxPollInterval <= 0 {
		return config{}, fmt.Errorf(
			"ctxscope: maximum poll interval must be greater than zero: %s",
			cfg.maxPollInterval,
		)
	}

	if cfg.startupTimeout < 0 {
		return config{}, fmt.Errorf(
			"ctxscope: startup timeout must not be negative: %s",
			cfg.startupTimeout,
		)
	}

	if cfg.maxPollInterval < cfg.pollInterval {
		cfg.maxPollInterval = cfg.pollInterval
	}

	return cfg, nil
}
