package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EventListRecordType represents the API resource type for EventSet records.
const EventListRecordType = "results.tekton.dev/v1.EventList"

// LogRecordType represents the API resource type for Tekton log records.
const LogRecordType = "results.tekton.dev/v1alpha3.Log"

// LogRecordTypeV2 represents the API resource type for Tekton log records deprecated now.
const LogRecordTypeV2 = "results.tekton.dev/v1alpha2.Log"

// Log represents the API resource for Tekton results Log.
type Log struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LogSpec   `json:"spec"`
	Status LogStatus `json:"status,omitempty"`
}

// LogSpec represents the specification for the Tekton Log resource.
// It contains information about corresponding log resource.
type LogSpec struct {
	Resource Resource `json:"resource"`
	Type     LogType  `json:"type"`
}

// Resource represents information to identify a Kubernetes API resource.
// It should be used to match the corresponding log to this resource.
type Resource struct {
	Kind      string    `json:"kind,omitempty"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	UID       types.UID `json:"uid,omitempty"`
}

// LogType represents the log storage type.
// This information is useful to determine how the resource will be stored.
type LogType string

const (
	// FileLogType defines the log type for logs stored in the file system.
	FileLogType LogType = "File"

	// S3LogType defines the log type for logs stored in the S3 object storage or S3 compatible alternatives.
	S3LogType LogType = "S3"

	// GCSLogType defines the log type for logs stored in the GCS object storage or GCS compatible alternatives.
	GCSLogType LogType = "GCS"

	// LokiLogType defines the log type for logs stored in the Loki.
	LokiLogType LogType = "loki"

	// BlobLogType defines the log type for logs stored in the Blob - GCS, S3 compatible storage.
	BlobLogType LogType = "blob"

	// SplunkLogType defines the log type for logs stored in the Splunk.
	SplunkLogType LogType = "splunk"
)

// LogStatus defines the current status of the log resource.
type LogStatus struct {
	Path            string `json:"path,omitempty"`
	Size            int64  `json:"size"`
	IsStored        bool   `json:"isStored"`
	ErrorOnStoreMsg string `json:"errorOnStoreMsg"`
	IsRetryableErr  bool   `json:"isRetryableErr"`
}

// Default sets up default values for Log TypeMeta, such as API version and kind.
func (t *Log) Default() {
	t.TypeMeta.Kind = "Log"
	t.TypeMeta.APIVersion = "results.tekton.dev/v1alpha3"
}
