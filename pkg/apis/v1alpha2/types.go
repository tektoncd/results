package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	TaskRunLogRecordType = "results.tekton.dev/v1alpha2.TaskRunLog"
)

type TaskRunLog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskRunLogSpec   `json:"spec"`
	Status TaskRunLogStatus `json:"status,omitempty"`
}

type TaskRunLogSpec struct {
	Ref        TaskRunRef     `json:"ref"`
	RecordName string         `json:"recordName"`
	Type       TaskRunLogType `json:"type"`
}

type TaskRunRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type TaskRunLogType string

const (
	FileLogType TaskRunLogType = "File"
)

type TaskRunLogStatus struct {
	File *FileLogTypeStatus `json:"file,omitempty"`
}

type FileLogTypeStatus struct {
	Path string `json:"path,omitempty"`
	Size int64  `json:"size"`
}

func (t *TaskRunLog) Default() {
	t.TypeMeta.Kind = "TaskRunLog"
	t.TypeMeta.APIVersion = "results.tekton.dev/v1alpha2"
}
