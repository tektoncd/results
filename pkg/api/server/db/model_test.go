package db

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAnnotationsScan(t *testing.T) {
	v := make(Annotations)
	v["foo"] = "bar"

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}

	var ann *Annotations
	if err := ann.Scan(bytes); err == nil {
		t.Error("annotation pointer must not be nil, expected error")
	}

	ann = &Annotations{}
	if err := ann.Scan(bytes); err != nil {
		t.Fatalf("failed to scan data from database: %v", err)
	}

	if diff := cmp.Diff(*ann, v); diff != "" {
		t.Errorf("-want, +got: %s", diff)
	}
}

func TestAnnotationsValue(t *testing.T) {
	v := make(Annotations)
	v["foo"] = "bar"

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}

	annv, err := v.Value()
	if err != nil {
		t.Fatalf("Value(): %v", err)
	}

	if diff := cmp.Diff(annv, bytes); diff != "" {
		t.Errorf("-want, +got: %s", diff)
	}
}
