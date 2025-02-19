package features

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// Feature is a string representation of a set of enabled/disabled features.
type Feature string

const (
	// PartialResponse feature enables response filtering
	PartialResponse Feature = "PartialResponse"
)

var defaultFeatures = map[Feature]bool{
	PartialResponse: false,
}

// FeatureGate provides a interface to manipulate features.
type FeatureGate interface {
	// Get returns true if the key is enabled.
	Get(f Feature) *atomic.Bool
	// Set parses and stores flag gates for known features
	// from a string like feature1=true,feature2=false,...
	Set(value string) error
	// String returns a string representation of the known features
	// in the following form feature1=true,feature2=false,...
	String() string
	// Enable will enable a feature
	Enable(f Feature)
	// Disable will disable a feature
	Disable(f Feature)
}

type featureGate map[Feature]*atomic.Bool

// NewFeatureGate creates a new FeatureGate
func NewFeatureGate() FeatureGate {
	fg := featureGate{}
	for k, v := range defaultFeatures {
		a := new(atomic.Bool)
		a.Store(v)
		fg[k] = a
	}
	return fg
}

// Get returns if a feature is enabled
func (fg featureGate) Get(f Feature) *atomic.Bool {
	if v, ok := fg[f]; ok {
		return v
	}
	return nil
}

// Set parses an input string and sets the FeatureGate
func (fg featureGate) Set(value string) error {
	value = strings.TrimSpace(value)
	for _, s := range strings.Split(value, ",") {
		if len(s) == 0 {
			continue
		}
		pair := strings.SplitN(s, "=", 2)
		f := Feature(pair[0])
		v := pair[1]
		if len(pair) != 2 {
			return fmt.Errorf("missing value for feature %s", f)
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid value of %s=%s, err: %v", f, v, err)
		}
		if v, ok := fg[f]; ok {
			v.Swap(b)
		} else {
			return fmt.Errorf("feature '%s' is not supported", f)
		}
	}
	return nil
}

// Disable disables a feature
func (fg featureGate) Disable(f Feature) {
	if v, ok := fg[f]; ok {
		v.Swap(false)
	}
}

// Enable enables a feature
func (fg featureGate) Enable(f Feature) {
	if v, ok := fg[f]; ok {
		v.Swap(true)
	}
}

// String returns the value of the FeatureGate as string representation
func (fg featureGate) String() string {
	var pairs []string
	for k, v := range fg {
		pairs = append(pairs, fmt.Sprintf("%s=%t", k, v.Load()))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}
