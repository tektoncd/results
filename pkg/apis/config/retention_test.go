package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

func defaultNamespaceCleanup() NamespaceCleanup {
	return NamespaceCleanup{
		InactivityPeriodDuration: DefaultNamespaceCleanupInactivityPeriod,
		MaxNamespacesPerRun:      DefaultNamespaceCleanupMaxPerRun,
	}
}

func TestNewRetentionPolicyFromConfigMap(t *testing.T) {
	type args struct {
		config *corev1.ConfigMap
	}
	tests := []struct {
		name    string
		args    args
		want    *RetentionPolicy
		wantErr bool
	}{
		{
			name: "empty config",
			args: args{config: &corev1.ConfigMap{}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				NamespaceCleanup: defaultNamespaceCleanup(),
				DefaultRetention: DefaultDefaultRetention,
			},
		},
		{
			name: "defaultRetention with d suffix",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"defaultRetention": "10d",
				},
			}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				NamespaceCleanup: defaultNamespaceCleanup(),
				DefaultRetention: 10 * 24 * time.Hour,
			},
		},
		{
			name: "defaultRetention without suffix",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"defaultRetention": "10",
				},
			}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				NamespaceCleanup: defaultNamespaceCleanup(),
				DefaultRetention: 10 * 24 * time.Hour,
			},
		},
		{
			name: "maxRetention(deprecated) without suffix",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"maxRetention": "10",
				},
			}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				NamespaceCleanup: defaultNamespaceCleanup(),
				DefaultRetention: 10 * 24 * time.Hour,
			},
		},
		{
			name: "maxRetention overrides defaultRetention for backward compatibility",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"defaultRetention": "30",
					"maxRetention":     "15",
				},
			}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				NamespaceCleanup: defaultNamespaceCleanup(),
				DefaultRetention: 15 * 24 * time.Hour,
			},
		},
		{
			name: "with policies",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"policies": `
- name: "policy1"
  selector:
    matchLabels:
      "app": ["foo"]
  retention: "10d"
`,
				},
			}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				NamespaceCleanup: defaultNamespaceCleanup(),
				DefaultRetention: DefaultDefaultRetention,
				Policies: []Policy{
					{
						Name: "policy1",
						Selector: Selector{
							MatchLabels: map[string][]string{"app": {"foo"}},
						},
						Retention: "10d",
					},
				},
			},
		},
		{
			name: "invalid policies yaml",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"policies": `
- name: "policy1"
  selector:
    matchLabels:
      "app": ["foo"]
  retention: "10d"
 :
`,
				},
			}},
			wantErr: true,
		},
		{
			name: "namespace cleanup enabled with defaults",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"namespaceCleanup": "enabled: true\n",
				},
			}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				DefaultRetention: DefaultDefaultRetention,
				NamespaceCleanup: NamespaceCleanup{
					Enabled:                  true,
					InactivityPeriodDuration: DefaultNamespaceCleanupInactivityPeriod,
					MaxNamespacesPerRun:      DefaultNamespaceCleanupMaxPerRun,
				},
			},
		},
		{
			name: "namespace cleanup fully configured",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"namespaceCleanup": `
enabled: true
inactivityPeriod: "48h"
excludeNamespaces:
  - "kube-system"
  - "default"
maxNamespacesPerRun: 5
dryRun: true
`,
				},
			}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				DefaultRetention: DefaultDefaultRetention,
				NamespaceCleanup: NamespaceCleanup{
					Enabled:                  true,
					InactivityPeriod:         "48h",
					ExcludeNamespaces:        []string{"kube-system", "default"},
					MaxNamespacesPerRun:      5,
					DryRun:                   true,
					InactivityPeriodDuration: 48 * time.Hour,
				},
			},
		},
		{
			name: "namespace cleanup inactivityPeriod without suffix is days",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"namespaceCleanup": "enabled: true\ninactivityPeriod: \"2\"\n",
				},
			}},
			want: &RetentionPolicy{
				RunAt:            DefaultRunAt,
				DefaultRetention: DefaultDefaultRetention,
				NamespaceCleanup: NamespaceCleanup{
					Enabled:                  true,
					InactivityPeriod:         "2",
					InactivityPeriodDuration: 2 * 24 * time.Hour,
					MaxNamespacesPerRun:      DefaultNamespaceCleanupMaxPerRun,
				},
			},
		},
		{
			name: "unknown namespace cleanup field is rejected",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"namespaceCleanup": "enabled: true\ndryrun: true\n",
				},
			}},
			wantErr: true,
		},
		{
			name: "invalid namespace cleanup yaml",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"namespaceCleanup": "enabled: true\n :\n",
				},
			}},
			wantErr: true,
		},
		{
			name: "invalid namespace cleanup inactivityPeriod",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"namespaceCleanup": "enabled: true\ninactivityPeriod: \"soon\"\n",
				},
			}},
			wantErr: true,
		},
		{
			name: "invalid namespace cleanup maxNamespacesPerRun",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"namespaceCleanup": "enabled: true\nmaxNamespacesPerRun: 0\n",
				},
			}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRetentionPolicyFromConfigMap(tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRetentionPolicyFromConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("NewRetentionPolicyFromConfigMap() = %v, want %v, diff: %s", got, tt.want, diff)
			}
		})
	}
}
