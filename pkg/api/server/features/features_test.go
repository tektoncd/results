package features

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func init() {
	defaultFeatures = map[Feature]bool{
		"feature-1": false,
		"feature-2": true,
	}
}

func TestNewFeatureGate(t *testing.T) {
	want := "feature-1=false,feature-2=true"
	got := NewFeatureGate().String()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FeatureGate data mismatch (-want +got):\n%s", diff)
	}
}

func TestFeatureGate_Get(t *testing.T) {
	want := defaultFeatures["feature-1"]
	got := NewFeatureGate().Get("feature-1").Load()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FeatureGate value mismatch (-want +got):\n%s", diff)
	}
}

func TestFeatureGate_Set(t *testing.T) {
	want := "feature-1=true,feature-2=false"
	f := NewFeatureGate()
	f.Set(want)
	got := f.String()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FeatureGate data mismatch (-want +got):\n%s", diff)
	}
}

func TestFeatureGate_Enable(t *testing.T) {
	f := NewFeatureGate()
	f.Enable("feature-1")
	want := true
	got := f.Get("feature-1").Load()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FeatureGate value mismatch (-want +got):\n%s", diff)
	}
}

func TestFeatureGate_Disable(t *testing.T) {
	f := NewFeatureGate()
	f.Disable("feature-2")
	want := false
	got := f.Get("feature-2").Load()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FeatureGate value mismatch (-want +got):\n%s", diff)
	}
}
