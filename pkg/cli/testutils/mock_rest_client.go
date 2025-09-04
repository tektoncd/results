package testutils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/cli/client"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/client-go/transport"
)

// filterRecordsByNamespace filters records based on the namespace in the URL path
func filterRecordsByNamespace(records []*pb.Record, urlPath string) []*pb.Record {
	// Extract parent from URL path like "/parents/{parent}/results/-/records"
	// parent can be:
	// - "namespace" for specific namespace (e.g., "production", "default")
	// - "-" for all namespaces

	parts := strings.Split(urlPath, "/")
	var parent string
	for i, part := range parts {
		if part == "parents" && i+1 < len(parts) {
			parent = parts[i+1]
			break
		}
	}

	// If parent is "-", return all records (all namespaces mode)
	if parent == "-" {
		return records
	}

	// Parent is the namespace directly (URL structure: /parents/{namespace}/results/-/records)
	targetNamespace := parent

	// Filter records by namespace
	var filteredRecords []*pb.Record
	for _, record := range records {
		// Parse the PipelineRun data to get the namespace
		var pipelineRun v1.PipelineRun
		if err := json.Unmarshal(record.Data.Value, &pipelineRun); err == nil {
			if pipelineRun.Namespace == targetNamespace {
				filteredRecords = append(filteredRecords, record)
			}
		}
	}

	return filteredRecords
}

// filterRecordsByPipelineName filters records based on pipeline name from the filter query parameter
func filterRecordsByPipelineName(records []*pb.Record, filterQuery string) []*pb.Record {
	// Parse the filter query to extract pipeline name filter
	// Example filter: "(data_type==\"tekton.dev/v1.PipelineRun\" || data_type==\"tekton.dev/v1beta1.PipelineRun\") && data.metadata.name.contains(\"build-pipeline\")"

	// Look for the pattern: data.metadata.name.contains("pipeline-name")
	nameFilterPrefix := "data.metadata.name.contains(\""
	nameFilterSuffix := "\")"

	startIdx := strings.Index(filterQuery, nameFilterPrefix)
	if startIdx == -1 {
		// No pipeline name filter found, return all records
		return records
	}

	startIdx += len(nameFilterPrefix)
	endIdx := strings.Index(filterQuery[startIdx:], nameFilterSuffix)
	if endIdx == -1 {
		// Malformed filter, return all records
		return records
	}

	pipelineName := filterQuery[startIdx : startIdx+endIdx]

	// Filter records by pipeline name (contains match)
	var filteredRecords []*pb.Record
	for _, record := range records {
		// Parse the PipelineRun data to get the name
		var pipelineRun v1.PipelineRun
		if err := json.Unmarshal(record.Data.Value, &pipelineRun); err == nil {
			if strings.Contains(pipelineRun.Name, pipelineName) {
				filteredRecords = append(filteredRecords, record)
			}
		}
	}

	return filteredRecords
}

// filterRecordsByLabels filters records based on label filters from the filter query parameter
func filterRecordsByLabels(records []*pb.Record, filterQuery string) []*pb.Record {
	// Parse the filter query to extract label filters
	// Example filter: "data.metadata.labels[\"app\"]==\"myapp\" && data.metadata.labels[\"env\"]==\"prod\""

	// Look for the pattern: data.metadata.labels["key"]=="value"
	labelFilterPrefix := "data.metadata.labels[\""
	labelFilterSuffix := "\"]=="

	// Extract all label filters from the query
	var labelFilters []struct {
		key   string
		value string
	}

	searchStart := 0
	for {
		startIdx := strings.Index(filterQuery[searchStart:], labelFilterPrefix)
		if startIdx == -1 {
			break
		}
		startIdx += searchStart + len(labelFilterPrefix)

		// Find the end of the key
		keyEndIdx := strings.Index(filterQuery[startIdx:], labelFilterSuffix)
		if keyEndIdx == -1 {
			break
		}

		key := filterQuery[startIdx : startIdx+keyEndIdx]
		valueStartIdx := startIdx + keyEndIdx + len(labelFilterSuffix)

		// Find the value (enclosed in quotes)
		if valueStartIdx >= len(filterQuery) || filterQuery[valueStartIdx] != '"' {
			break
		}
		valueStartIdx++ // Skip opening quote

		valueEndIdx := strings.Index(filterQuery[valueStartIdx:], "\"")
		if valueEndIdx == -1 {
			break
		}

		value := filterQuery[valueStartIdx : valueStartIdx+valueEndIdx]
		labelFilters = append(labelFilters, struct {
			key   string
			value string
		}{key: key, value: value})

		searchStart = valueStartIdx + valueEndIdx + 1
	}

	// If no label filters found, return all records
	if len(labelFilters) == 0 {
		return records
	}

	// Filter records by labels (all labels must match)
	var filteredRecords []*pb.Record
	for _, record := range records {
		// Parse the PipelineRun data to get the labels
		var pipelineRun v1.PipelineRun
		if err := json.Unmarshal(record.Data.Value, &pipelineRun); err == nil {
			// Check if all label filters match
			allMatch := true
			for _, labelFilter := range labelFilters {
				if pipelineRun.Labels == nil {
					allMatch = false
					break
				}
				if labelValue, exists := pipelineRun.Labels[labelFilter.key]; !exists || labelValue != labelFilter.value {
					allMatch = false
					break
				}
			}
			if allMatch {
				filteredRecords = append(filteredRecords, record)
			}
		}
	}

	return filteredRecords
}

// MockRESTClientFromRecords creates a mock REST client with comprehensive filtering support
func MockRESTClientFromRecords(records []*pb.Record) (*client.RESTClient, error) {
	// Create HTTP server that handles multiple endpoints with filtering
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records"):
			// Handle ListRecords with namespace and pipeline name filtering
			filteredRecords := filterRecordsByNamespace(records, r.URL.Path)

			// Apply filtering from query parameters
			if filter := r.URL.Query().Get("filter"); filter != "" {
				filteredRecords = filterRecordsByPipelineName(filteredRecords, filter)
				filteredRecords = filterRecordsByLabels(filteredRecords, filter)
			}

			// Create response with filtered records (no pagination)
			grpcResp := &pb.ListRecordsResponse{
				Records:       filteredRecords,
				NextPageToken: "", // No pagination support
			}
			jsonData, err := protojson.Marshal(grpcResp)
			if err != nil {
				http.Error(w, "Failed to marshal records response", http.StatusInternalServerError)
				return
			}
			if _, err := w.Write(jsonData); err != nil {
				http.Error(w, "Failed to write response", http.StatusInternalServerError)
			}
			return

		case r.Method == "GET" && strings.Contains(r.URL.Path, "/logs"):
			// Handle logs endpoints - can be extended as needed
			// For now, return empty response
			logsResp := &pb.ListRecordsResponse{Records: nil}
			jsonData, err := protojson.Marshal(logsResp)
			if err != nil {
				http.Error(w, "Failed to marshal logs response", http.StatusInternalServerError)
				return
			}
			if _, err := w.Write(jsonData); err != nil {
				http.Error(w, "Failed to write response", http.StatusInternalServerError)
			}
			return
		}

		http.NotFound(w, r)
	}))

	serverURL, err := url.Parse(server.URL + "/apis/results.tekton.dev/v1alpha2")
	if err != nil {
		return nil, fmt.Errorf("failed to parse server URL: %w", err)
	}

	config := &client.Config{
		URL:       serverURL,
		Timeout:   30 * time.Second,
		Transport: &transport.Config{},
	}

	restClient, err := client.NewRESTClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}

	return restClient, nil
}
