package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

const (
	runAt           = "runAt"
	maxRetention    = "maxRetention"
	policiesKey     = "policies"
	retentionCMName = "tekton-results-config-results-retention-policy"
	// DefaultRunAt is the default value for RunAt
	DefaultRunAt = "7 7 * * 7"
	// DefaultMaxRetention is the default value for MaxRetention
	DefaultMaxRetention = time.Hour * 24 * 30
)

// ParseDuration parses a string into a time.Duration.
// It handles standard formats like "24h", "90m", as well as the "d" suffix for days.
// If no unit is specified, it defaults to days.
func ParseDuration(durationStr string) (time.Duration, error) {
	if strings.HasSuffix(durationStr, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(durationStr, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	if days, err := strconv.Atoi(durationStr); err == nil {
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(durationStr)
}

// Policy defines a single retention policy rule.
type Policy struct {
	Name      string   `yaml:"name"`
	Selector  Selector `yaml:"selector"`
	Retention string   `yaml:"retention"`
}

// Selector defines the selection criteria for a policy.
type Selector struct {
	MatchNamespace   []string            `yaml:"matchNamespace"`
	MatchLabels      map[string][]string `yaml:"matchLabels"`
	MatchAnnotations map[string][]string `yaml:"matchAnnotations"`
	Status           []string            `yaml:"status"`
}

// RetentionPolicy holds the configurations for the Retention Policy of the DB
type RetentionPolicy struct {
	RunAt        string
	MaxRetention time.Duration
	Policies     []Policy
}

// DeepCopy copying the receiver, creating a new RetentionPolicy.
// deepcopy-gen hasn't been introduced in results repo, so handcraft here for now
func (cfg *RetentionPolicy) DeepCopy() *RetentionPolicy {
	return &RetentionPolicy{
		RunAt:        cfg.RunAt,
		MaxRetention: cfg.MaxRetention,
		Policies:     cfg.Policies,
	}
}

// Equals returns true if two Configs are identical
func (cfg *RetentionPolicy) Equals(other *RetentionPolicy) bool {
	if cfg == nil && other == nil {
		return true
	}

	if cfg == nil || other == nil {
		return false
	}

	return other.RunAt == cfg.RunAt &&
		other.MaxRetention == cfg.MaxRetention
}

func newRetentionPolicyFromMap(cfgMap map[string]string) (*RetentionPolicy, error) {
	rp := RetentionPolicy{
		RunAt:        DefaultRunAt,
		MaxRetention: DefaultMaxRetention,
	}

	if schedule, ok := cfgMap[runAt]; ok {
		rp.RunAt = schedule
	}

	if duration, ok := cfgMap[maxRetention]; ok {
		v, err := ParseDuration(duration)
		if err != nil {
			return nil, fmt.Errorf("incorrect configuration for maxRetention: %w", err)
		}
		rp.MaxRetention = v
	}

	if policiesYAML, ok := cfgMap[policiesKey]; ok {
		var policies []Policy
		if err := yaml.Unmarshal([]byte(policiesYAML), &policies); err != nil {
			return nil, fmt.Errorf("failed to unmarshal policies: %w", err)
		}
		rp.Policies = policies
	}

	return &rp, nil
}

// NewRetentionPolicyFromConfigMap returns a Config for the given configmap
func NewRetentionPolicyFromConfigMap(config *corev1.ConfigMap) (*RetentionPolicy, error) {
	return newRetentionPolicyFromMap(config.Data)
}

// GetRetentionPolicyConfigName returns the name of the configmap containing
// retention policy.
func GetRetentionPolicyConfigName() string {
	if e := os.Getenv("CONFIG_RETENTION_POLICY_NAME"); e != "" {
		return e
	}
	return retentionCMName
}
