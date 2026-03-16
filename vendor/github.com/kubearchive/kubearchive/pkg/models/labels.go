// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// LabelFilters encapsulates the database filters for labels in the archived resources
type LabelFilters struct {
	Exists    []string
	NotExists []string
	Equals    map[string]string
	NotEquals map[string]string
	In        map[string][]string
	NotIn     map[string][]string
}

func NewLabelFilters(labelRequirements []labels.Requirement) (*LabelFilters, error) {
	lf := LabelFilters{}
	var err error
	for _, r := range labelRequirements {
		switch r.Operator() {
		case selection.Exists:
			if lf.Exists == nil {
				lf.Exists = []string{}
			}
			lf.Exists = append(lf.Exists, r.Key())
		case selection.DoesNotExist:
			if lf.NotExists == nil {
				lf.NotExists = []string{}
			}
			lf.NotExists = append(lf.NotExists, r.Key())
		case selection.Equals:
			if lf.Equals == nil {
				lf.Equals = make(map[string]string)
			}
			lf.Equals[r.Key()] = r.ValuesUnsorted()[0]
		case selection.NotEquals:
			if lf.NotEquals == nil {
				lf.NotEquals = make(map[string]string)
			}
			lf.NotEquals[r.Key()] = r.ValuesUnsorted()[0]
		case selection.In:
			if lf.In == nil {
				lf.In = make(map[string][]string)
			}
			lf.In[r.Key()] = r.ValuesUnsorted()
		case selection.NotIn:
			if lf.NotIn == nil {
				lf.NotIn = make(map[string][]string)
			}
			lf.NotIn[r.Key()] = r.ValuesUnsorted()
		default:
			err = fmt.Errorf("unsupported label filter %s", r.Operator())
			return nil, err
		}
	}
	return &lf, nil
}
