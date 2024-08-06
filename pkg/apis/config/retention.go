package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	runAt           = "runAt"
	maxRetention    = "maxRetention"
	retentionCMName = "tekton-results-config-results-retention-policy"
	// DefaultRunAt is the default value for RunAt
	DefaultRunAt = "7 7 * * 7"
	// DefaultMaxRetention is the default value for MaxRetention
	DefaultMaxRetention = time.Hour * 24 * 30
)

// RetentionPolicy holds the configurations for the Retention Policy of the DB
type RetentionPolicy struct {
	RunAt        string
	MaxRetention time.Duration
}

// DeepCopy copying the receiver, creating a new RetentionPolicy.
// deepcopy-gen hasn't been introduced in results repo, so handcraft here for now
func (cfg *RetentionPolicy) DeepCopy() *RetentionPolicy {
	return &RetentionPolicy{
		RunAt:        cfg.RunAt,
		MaxRetention: cfg.MaxRetention,
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
		v, err := strconv.Atoi(duration)
		if err != nil {
			return nil, fmt.Errorf("incorrect configuration for maxRetention:%s", err.Error())
		}
		rp.MaxRetention = time.Hour * 24 * time.Duration(v)
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
