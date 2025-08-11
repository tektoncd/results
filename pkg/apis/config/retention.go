package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

const (
	runAt            = "runAt"
	maxRetention     = "maxRetention"
	defaultRetention = "defaultRetention"
	policiesKey      = "policies"
	retentionCMName  = "tekton-results-config-results-retention-policy"
	// DefaultRunAt is the default value for RunAt
	DefaultRunAt = "7 7 * * 7"
	// DefaultDefaultRetention is the default value for DefaultRetention
	DefaultDefaultRetention = time.Hour * 24 * 30
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
	MatchNamespaces  []string            `yaml:"matchNamespaces"`
	MatchLabels      map[string][]string `yaml:"matchLabels"`
	MatchAnnotations map[string][]string `yaml:"matchAnnotations"`
	MatchStatuses    []string            `yaml:"matchStatuses"`
}

// RetentionPolicy holds the configurations for the Retention Policy of the DB
type RetentionPolicy struct {
	RunAt            string
	DefaultRetention time.Duration
	Policies         []Policy
}

// DeepCopy copying the receiver, creating a new RetentionPolicy.
// deepcopy-gen hasn't been introduced in results repo, so handcraft here for now
func (cfg *RetentionPolicy) DeepCopy() *RetentionPolicy {
	if cfg == nil {
		return nil
	}
	newCfg := &RetentionPolicy{
		RunAt:            cfg.RunAt,
		DefaultRetention: cfg.DefaultRetention,
	}
	if cfg.Policies != nil {
		newCfg.Policies = make([]Policy, len(cfg.Policies))
		for i, p := range cfg.Policies {
			newCfg.Policies[i] = *p.DeepCopy()
		}
	}
	return newCfg
}

// DeepCopy returns a deep copy of the Policy.
func (p *Policy) DeepCopy() *Policy {
	if p == nil {
		return nil
	}
	return &Policy{
		Name:      p.Name,
		Selector:  *p.Selector.DeepCopy(),
		Retention: p.Retention,
	}
}

// DeepCopy returns a deep copy of the Selector.
func (s *Selector) DeepCopy() *Selector {
	if s == nil {
		return nil
	}
	out := &Selector{}
	if s.MatchNamespaces != nil {
		out.MatchNamespaces = append([]string(nil), s.MatchNamespaces...)
	}
	if s.MatchStatuses != nil {
		out.MatchStatuses = append([]string(nil), s.MatchStatuses...)
	}
	if s.MatchLabels != nil {
		out.MatchLabels = make(map[string][]string, len(s.MatchLabels))
		for k, v := range s.MatchLabels {
			out.MatchLabels[k] = append([]string(nil), v...)
		}
	}
	if s.MatchAnnotations != nil {
		out.MatchAnnotations = make(map[string][]string, len(s.MatchAnnotations))
		for k, v := range s.MatchAnnotations {
			out.MatchAnnotations[k] = append([]string(nil), v...)
		}
	}
	return out
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
		other.DefaultRetention == cfg.DefaultRetention
}

func newRetentionPolicyFromMap(cfgMap map[string]string) (*RetentionPolicy, error) {
	rp := RetentionPolicy{
		RunAt:            DefaultRunAt,
		DefaultRetention: DefaultDefaultRetention,
	}

	if schedule, ok := cfgMap[runAt]; ok {
		rp.RunAt = schedule
	}

	if duration, ok := cfgMap[defaultRetention]; ok {
		v, err := ParseDuration(duration)
		if err != nil {
			return nil, fmt.Errorf("incorrect configuration for defaultRetention: %w", err)
		}
		rp.DefaultRetention = v
	} else if duration, ok := cfgMap[maxRetention]; ok {
		log.Println("WARNING: configuration key 'maxRetention' is deprecated; please use 'defaultRetention' instead.")
		v, err := ParseDuration(duration)
		if err != nil {
			return nil, fmt.Errorf("incorrect configuration for maxRetention: %w", err)
		}
		rp.DefaultRetention = v
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
