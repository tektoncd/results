package config

import (
	"context"

	"knative.dev/pkg/configmap"
)

type cfgKey struct{}

// Config holds the collection of configurations that we attach to contexts.
type Config struct {
	Metrics *Metrics
}

// FromContext extracts a Config from the provided context.
func FromContext(ctx context.Context) *Config {
	x, ok := ctx.Value(cfgKey{}).(*Config)
	if ok {
		return x
	}
	return nil
}

// ToContext attaches the provided Config to the provided context, returning the
// new context with the Config attached.
func ToContext(ctx context.Context, c *Config) context.Context {
	return context.WithValue(ctx, cfgKey{}, c)
}

// Store is a typed wrapper around configmap.Untyped store to handle our configmaps.
type Store struct {
	*configmap.UntypedStore
}

// NewStore creates a new store of Configs and optionally calls functions when ConfigMaps are updated.
func NewStore(logger configmap.Logger, onAfterStore ...func(name string, value any)) *Store {
	store := &Store{
		UntypedStore: configmap.NewUntypedStore(
			"results",
			logger,
			configmap.Constructors{
				GetMetricsConfigName():         NewMetricsFromConfigMap,
				GetRetentionPolicyConfigName(): NewRetentionPolicyFromConfigMap,
			},
			onAfterStore...,
		),
	}

	return store
}

// ToContext attaches the current Config state to the provided context.
func (s *Store) ToContext(ctx context.Context) context.Context {
	return ToContext(ctx, s.Load())
}

// Load creates a Config from the current config state of the Store.
func (s *Store) Load() *Config {
	metrics := s.UntypedLoad(GetMetricsConfigName())
	if metrics == nil {
		metrics, _ = newMetricsFromMap(map[string]string{})
	}
	return &Config{
		Metrics: metrics.(*Metrics).DeepCopy(),
	}
}
