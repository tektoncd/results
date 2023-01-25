package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	LogRecordType = "results.tekton.dev/v1alpha2.Log"
)

type Log struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LogSpec   `json:"spec"`
	Status LogStatus `json:"status,omitempty"`
}

type LogSpec struct {
	Resource Resource `json:"resource"`
	Type     LogType  `json:"type"`
}

type Resource struct {
	Kind      string    `json:"kind,omitempty"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	UID       types.UID `json:"uid,omitempty"`
}

type LogType string

const (
	FileLogType LogType = "File"
	S3LogType   LogType = "S3"
)

type LogStatus struct {
	Path string `json:"path,omitempty"`
	Size int64  `json:"size"`
}

func (t *Log) Default() {
	t.TypeMeta.Kind = "Log"
	t.TypeMeta.APIVersion = "results.tekton.dev/v1alpha2"
}
