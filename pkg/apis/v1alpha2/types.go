package v1alpha2

const (
	TaskRunLogRecordType = "results.tekton.dev/v1alpha2.TaskRunLog"
)

type TaskRunLog struct {
	Type TaskRunLogType   `json:"type"`
	File *FileLogTypeSpec `json:"file,omitempty"`
}

type TaskRunLogType string

const (
	FileLogType TaskRunLogType = "File"
)

type FileLogTypeSpec struct {
	Path string `json:"path"`
}
