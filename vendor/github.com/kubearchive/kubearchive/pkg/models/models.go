// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"database/sql"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Unmarshal event.Data() to k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured. If the cloudevent data
// cannot be unmarshalled, return the error
func UnstructuredFromByteSlice(data []byte) (*unstructured.Unstructured, error) {
	k8sObj := &unstructured.Unstructured{}
	err := k8sObj.UnmarshalJSON(data)
	return k8sObj, err
}

// converts time.Time to string in RFC3339 format with the timezone set to UTC to match how Kubernetes displays
// timestamps itself
func FormatTimestamp(timestamp time.Time) string {
	return timestamp.UTC().Format(time.RFC3339)
}

// Create an RFC3339 Timestamp from a *metav1.Time that might be nil
func OptionalTimestamp(timestamp *metav1.Time) sql.NullString {
	if timestamp == nil {
		return sql.NullString{
			String: "",
			Valid:  false,
		}
	}
	return sql.NullString{
		String: FormatTimestamp(timestamp.Time),
		Valid:  true,
	}
}

type LogTuple struct {
	ContainerName string
	Url           string
	Query         string
	Start         string
	End           string
}

type Resource struct {
	Date string `db:"created_at"`
	Id   int64  `db:"id"`
	Uuid string `db:"uuid"`
	Data string `db:"data"`
}
