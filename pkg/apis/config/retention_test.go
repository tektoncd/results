package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

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
				RunAt:        DefaultRunAt,
				MaxRetention: DefaultMaxRetention,
			},
		},
		{
			name: "maxRetention with d suffix",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"maxRetention": "10d",
				},
			}},
			want: &RetentionPolicy{
				RunAt:        DefaultRunAt,
				MaxRetention: 10 * 24 * time.Hour,
			},
		},
		{
			name: "maxRetention without suffix",
			args: args{config: &corev1.ConfigMap{
				Data: map[string]string{
					"maxRetention": "10",
				},
			}},
			want: &RetentionPolicy{
				RunAt:        DefaultRunAt,
				MaxRetention: 10 * 24 * time.Hour,
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
				RunAt:        DefaultRunAt,
				MaxRetention: DefaultMaxRetention,
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
