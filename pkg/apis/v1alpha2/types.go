package v1alpha2

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
