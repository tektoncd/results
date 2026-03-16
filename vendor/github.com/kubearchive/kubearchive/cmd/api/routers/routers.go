// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/pagination"
	"github.com/kubearchive/kubearchive/pkg/abort"
	dbErrors "github.com/kubearchive/kubearchive/pkg/database/errors"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	labelFilter "github.com/kubearchive/kubearchive/pkg/models"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"k8s.io/apimachinery/pkg/labels"
)

type CacheExpirations struct {
	Authorized   time.Duration
	Unauthorized time.Duration
}

type Controller struct {
	Database           interfaces.DBReader
	CacheConfiguration CacheExpirations
}

func (c *Controller) GetResources(context *gin.Context) {
	limit, id, date := pagination.GetValuesFromContext(context)
	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	name := context.Param("name")

	if name != "" && strings.Contains(name, "*") {
		abort.Abort(context, errors.New("wildcard characters (*) are not allowed in path parameters, use query parameter ?name= instead"), http.StatusBadRequest)
		return
	}

	queryName := context.Query("name")

	if name != "" && queryName != "" {
		abort.Abort(context, errors.New("cannot specify both path name parameter and query name parameter"), http.StatusBadRequest)
		return
	}

	if queryName != "" {
		name = queryName
	}
	selector, parserErr := labels.Parse(context.Query("labelSelector"))
	if parserErr != nil {
		abort.Abort(context, parserErr, http.StatusBadRequest)
		return
	}
	reqs, _ := selector.Requirements()
	labelFilters, labelFiltersErr := labelFilter.NewLabelFilters(reqs)
	if labelFiltersErr != nil {
		abort.Abort(context, labelFiltersErr, http.StatusBadRequest)
	}

	if strings.HasPrefix(context.Request.URL.Path, "/apis/") && group == "" {
		abort.Abort(context, errors.New(http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	// Parse timestamp filters
	creationTimestampAfter, err := parseTimestampQuery(context, "creationTimestampAfter")
	if err != nil {
		abort.Abort(context, err, http.StatusBadRequest)
		return
	}

	creationTimestampBefore, err := parseTimestampQuery(context, "creationTimestampBefore")
	if err != nil {
		abort.Abort(context, err, http.StatusBadRequest)
		return
	}

	if creationTimestampAfter != nil && creationTimestampBefore != nil {
		if creationTimestampBefore.Before(*creationTimestampAfter) || creationTimestampBefore.Equal(*creationTimestampAfter) {
			abort.Abort(context, errors.New("creationTimestampBefore must be after creationTimestampAfter"), http.StatusBadRequest)
			return
		}
	}

	apiVersion := version
	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, version)
	}

	// Single resource by exact name - no streaming needed
	if name != "" && !strings.Contains(name, "*") {
		var resources []labelFilter.Resource
		resources, err = c.Database.QueryResources(
			context.Request.Context(), kind, apiVersion, namespace, name, id, date, labelFilters,
			creationTimestampAfter, creationTimestampBefore, 2)
		if err != nil {
			abort.Abort(context, err, http.StatusInternalServerError)
			return
		}
		if len(resources) == 0 {
			abort.Abort(context, errors.New("resource not found"), http.StatusNotFound)
			return
		} else if len(resources) > 1 {
			abort.Abort(context, errors.New("more than one resource found"), http.StatusInternalServerError)
			return
		}
		context.Data(http.StatusOK, "application/json", []byte(resources[0].Data))
		return
	}

	// List/wildcard case - stream the response directly to the writer to avoid
	// buffering the entire JSON payload in memory. This prevents OOM kills when
	// serving large result sets (e.g. limit=500 with large PipelineRuns).
	//
	// Items are written before metadata so the continue token can be determined
	// after iterating all rows. JSON key order does not affect parsers.
	newLimit := limit + 1
	var count int
	var lastWritten labelFilter.Resource
	var hasMore bool
	var headerWritten bool
	w := context.Writer

	err = c.Database.StreamResources(
		context.Request.Context(), kind, apiVersion, namespace, name, id, date, labelFilters,
		creationTimestampAfter, creationTimestampBefore, newLimit,
		func(resource labelFilter.Resource) error {
			count++
			if count > limit {
				hasMore = true
				return nil
			}
			if !headerWritten {
				context.Status(http.StatusOK)
				context.Header("Content-Type", "application/json")
				io.WriteString(w, `{"kind": "List", "apiVersion": "v1", "items": [`) //nolint:errcheck
				headerWritten = true
			} else {
				io.WriteString(w, ",") //nolint:errcheck
			}
			io.WriteString(w, resource.Data) //nolint:errcheck
			lastWritten = resource
			return nil
		})

	if err != nil {
		if !headerWritten {
			abort.Abort(context, err, http.StatusInternalServerError)
			return
		}
		// Response already started - log the error, client will see truncated JSON
		slog.ErrorContext(context.Request.Context(), "error streaming resources", "error", err)
		return
	}

	if !headerWritten {
		// No rows returned - write a complete empty response
		context.Status(http.StatusOK)
		context.Header("Content-Type", "application/json")
		io.WriteString(w, `{"kind": "List", "apiVersion": "v1", "items": [`) //nolint:errcheck
	}

	continueToken := ""
	if hasMore {
		continueToken = pagination.CreateToken(lastWritten.Id, lastWritten.Date)
	}
	fmt.Fprintf(w, `], "metadata": {"continue": "%s"}}`, continueToken) //nolint:errcheck
}

// parseTimestampQuery parses a timestamp query parameter and returns a pointer to time.Time
// Returns nil if the parameter is not provided or empty
func parseTimestampQuery(context *gin.Context, paramName string) (*time.Time, error) {
	value := context.Query(paramName)
	if value == "" {
		return nil, nil //nolint:nilnil // This is intentional - empty parameter means no filter
	}

	// Try parsing as RFC3339 format (ISO 8601)
	timestamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		// Try parsing as RFC3339Nano format
		timestamp, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, fmt.Errorf("invalid %s format: %s. Expected RFC3339 format (e.g., 2023-01-01T12:00:00Z)", paramName, value)
		}
	}

	return &timestamp, nil
}

func (c *Controller) GetLogURL(context *gin.Context) {
	var err error
	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	name := context.Param("name")
	uid := context.Param("uid")
	containerName := context.Query("container")

	if strings.HasPrefix(context.Request.URL.Path, "/apis/") && group == "" {
		abort.Abort(context, errors.New(http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	apiVersion := version
	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, version)
	}

	var logRecord *interfaces.LogRecord
	if name != "" {
		logRecord, err = c.Database.QueryLogURLByName(
			context.Request.Context(), kind, apiVersion, namespace, name, containerName)
	} else {
		logRecord, err = c.Database.QueryLogURLByUID(
			context.Request.Context(), kind, apiVersion, namespace, uid, containerName)
	}

	if errors.Is(err, dbErrors.ErrResourceNotFound) {
		abort.Abort(context, err, http.StatusNotFound)
		return
	}
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	context.Set("logRecord", logRecord)
}

func (c *Controller) GetResourceByUID(context *gin.Context) {
	kind, err := discovery.GetAPIResourceKind(context) // Not used but required for validation
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	uid := context.Param("uid")

	apiVersion := version
	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, version)
	}

	if strings.HasPrefix(context.Request.URL.Path, "/apis/") && group == "" {
		abort.Abort(context, errors.New(http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	resource, err := c.Database.QueryResourceByUID(context.Request.Context(), kind, apiVersion, namespace, uid)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	if resource == nil {
		abort.Abort(context, errors.New("resource not found"), http.StatusNotFound)
		return
	}

	context.String(http.StatusOK, resource.Data)
}

// Livez returns current server configuration as we don't have a clear deadlock indicator
func (c *Controller) Livez(context *gin.Context) {
	observabilityConfig := observability.Status()

	context.JSON(http.StatusOK, gin.H{
		"code":           http.StatusOK,
		"ginMode":        gin.Mode(),
		"authCacheTTL":   c.CacheConfiguration.Authorized,
		"unAuthCacheTTL": c.CacheConfiguration.Unauthorized,
		"openTelemetry":  observabilityConfig,
		"message":        "healthy",
	})
}

// Readyz checks Database connection
func (c *Controller) Readyz(context *gin.Context) {
	err := c.Database.Ping(context.Request.Context())
	if err != nil {
		abort.Abort(context, err, http.StatusServiceUnavailable)
		return
	}
	context.JSON(http.StatusOK, gin.H{"message": "ready"})
}
