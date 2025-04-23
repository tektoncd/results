package common

import (
	"fmt"
	"time"

	"github.com/jonboulle/clockwork"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FormatAge returns a human-readable string representation of how long ago a timestamp occurred.
// The output format varies based on duration: seconds (<1m), minutes (<1h), hours (<24h), or days.
func FormatAge(t *metav1.Time, c clockwork.Clock) string {
	if t == nil {
		return ""
	}
	age := c.Since(t.Time)
	if age < time.Minute {
		return fmt.Sprintf("%ds ago", int(age.Seconds()))
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	}
	if age < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(age.Hours()/24))
}
