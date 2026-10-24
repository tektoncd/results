/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package retention

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tektoncd/results/pkg/apis/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// logRecorder is a zapcore.Core capturing the logged messages of a given level.
type logRecorder struct {
	zapcore.LevelEnabler

	mutex    sync.Mutex
	messages []string
}

func newLogRecorder(level zapcore.Level) *logRecorder {
	return &logRecorder{LevelEnabler: level}
}

func (r *logRecorder) With([]zapcore.Field) zapcore.Core { return r }

func (r *logRecorder) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if r.Enabled(entry.Level) {
		return checked.AddCore(entry, r)
	}
	return checked
}

func (r *logRecorder) Write(entry zapcore.Entry, _ []zapcore.Field) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.messages = append(r.messages, entry.Message)
	return nil
}

func (r *logRecorder) Sync() error { return nil }

func (r *logRecorder) Messages() []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]string(nil), r.messages...)
}

// fakeNamespaceLister returns a canned list of namespaces.
type fakeNamespaceLister struct {
	namespaces []string
	err        error
}

func (f *fakeNamespaceLister) List(_ context.Context) ([]string, error) {
	return f.namespaces, f.err
}

// inactiveNamespacesQuery matches the query selecting the namespaces of the DB
// that haven't been updated within the inactivity period.
const inactiveNamespacesQuery = `(?s)SELECT\s+parent\s+FROM\s+results\s+GROUP\s+BY\s+parent\s+HAVING\s+MAX\(updated_time\)\s*<\s*NOW\(\)\s*-\s*INTERVAL\s*'86400\.000000\s+seconds'`

func newTestAgent(t *testing.T, cleanup config.NamespaceCleanup, lister namespaceLister) (*Agent, sqlmock.Sqlmock, func()) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("unexpected error when opening a stub database connection: %v", err)
	}

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: mockDB}), &gorm.Config{})
	if err != nil {
		mockDB.Close()
		t.Fatalf("unexpected error when opening a gorm database: %v", err)
	}

	agent := &Agent{
		Logger:     zap.NewNop().Sugar(),
		db:         gormDB,
		ctx:        context.Background(),
		namespaces: lister,
	}
	agent.RetentionPolicy = config.RetentionPolicy{NamespaceCleanup: cleanup}

	return agent, mock, func() { mockDB.Close() }
}

func defaultCleanupConfig() config.NamespaceCleanup {
	return config.NamespaceCleanup{
		Enabled:                  true,
		InactivityPeriodDuration: 24 * time.Hour,
		MaxNamespacesPerRun:      config.DefaultNamespaceCleanupMaxPerRun,
	}
}

func TestAgent_cleanupDeletedNamespaces(t *testing.T) {
	tests := []struct {
		name string
		// cleanup is the namespace cleanup configuration under test.
		cleanup config.NamespaceCleanup
		// lister returns the namespaces that exist in the cluster.
		lister *fakeNamespaceLister
		// dbNamespaces are the inactive namespaces stored in the DB. They are
		// not queried at all when nil.
		dbNamespaces []string
		// wantDeleted are the namespaces expected to be deleted, in order.
		wantDeleted []string
	}{
		{
			name:         "deletes the namespaces missing from the cluster",
			cleanup:      defaultCleanupConfig(),
			lister:       &fakeNamespaceLister{namespaces: []string{"live", "kube-system"}},
			dbNamespaces: []string{"live", "gone", "also-gone"},
			wantDeleted:  []string{"also-gone", "gone"},
		},
		{
			name:         "keeps the namespaces that still exist",
			cleanup:      defaultCleanupConfig(),
			lister:       &fakeNamespaceLister{namespaces: []string{"live", "other"}},
			dbNamespaces: []string{"live", "other"},
		},
		{
			name:         "keeps the namespaces still within the inactivity period",
			cleanup:      defaultCleanupConfig(),
			lister:       &fakeNamespaceLister{namespaces: []string{"live"}},
			dbNamespaces: nil,
		},
		{
			name: "keeps the excluded namespaces",
			cleanup: func() config.NamespaceCleanup {
				c := defaultCleanupConfig()
				c.ExcludeNamespaces = []string{"gone"}
				return c
			}(),
			lister:       &fakeNamespaceLister{namespaces: []string{"live"}},
			dbNamespaces: []string{"gone", "also-gone"},
			wantDeleted:  []string{"also-gone"},
		},
		{
			name:    "skips the cleanup when the cluster namespaces cannot be listed",
			cleanup: defaultCleanupConfig(),
			lister:  &fakeNamespaceLister{err: errors.New("api server is down")},
		},
		{
			name:    "skips the cleanup when the cluster reports no namespaces",
			cleanup: defaultCleanupConfig(),
			lister:  &fakeNamespaceLister{namespaces: nil},
		},
		{
			name: "skips the cleanup when too many namespaces are eligible",
			cleanup: func() config.NamespaceCleanup {
				c := defaultCleanupConfig()
				c.MaxNamespacesPerRun = 1
				return c
			}(),
			lister:       &fakeNamespaceLister{namespaces: []string{"live"}},
			dbNamespaces: []string{"gone", "also-gone"},
		},
		{
			name: "deletes nothing in dry run mode",
			cleanup: func() config.NamespaceCleanup {
				c := defaultCleanupConfig()
				c.DryRun = true
				return c
			}(),
			lister:       &fakeNamespaceLister{namespaces: []string{"live"}},
			dbNamespaces: []string{"gone"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, mock, closeDB := newTestAgent(t, tt.cleanup, tt.lister)
			defer closeDB()

			// The DB is only queried once the cluster namespaces are known.
			if tt.lister.err == nil && len(tt.lister.namespaces) > 0 {
				rows := sqlmock.NewRows([]string{"parent"})
				for _, ns := range tt.dbNamespaces {
					rows.AddRow(ns)
				}
				mock.ExpectQuery(inactiveNamespacesQuery).WillReturnRows(rows)
			}

			for _, ns := range tt.wantDeleted {
				mock.ExpectExec(`DELETE FROM results WHERE parent = \$1`).
					WithArgs(ns).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}

			agent.cleanupDeletedNamespaces(agent.snapshotConfig().NamespaceCleanup)

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestAgent_cleanupDeletedNamespaces_continuesOnDeleteError(t *testing.T) {
	agent, mock, closeDB := newTestAgent(t, defaultCleanupConfig(),
		&fakeNamespaceLister{namespaces: []string{"live"}})
	defer closeDB()

	mock.ExpectQuery(inactiveNamespacesQuery).
		WillReturnRows(sqlmock.NewRows([]string{"parent"}).AddRow("gone").AddRow("also-gone"))
	mock.ExpectExec(`DELETE FROM results WHERE parent = \$1`).
		WithArgs("also-gone").
		WillReturnError(errors.New("deadlock detected"))
	mock.ExpectExec(`DELETE FROM results WHERE parent = \$1`).
		WithArgs("gone").
		WillReturnResult(sqlmock.NewResult(0, 2))

	agent.cleanupDeletedNamespaces(agent.snapshotConfig().NamespaceCleanup)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestAgent_cleanupDeletedNamespaces_logsSkippedRun(t *testing.T) {
	core := newLogRecorder(zap.ErrorLevel)

	cleanup := defaultCleanupConfig()
	cleanup.MaxNamespacesPerRun = 1

	agent, mock, closeDB := newTestAgent(t, cleanup, &fakeNamespaceLister{namespaces: []string{"live"}})
	defer closeDB()
	agent.Logger = zap.New(core).Sugar()

	mock.ExpectQuery(inactiveNamespacesQuery).
		WillReturnRows(sqlmock.NewRows([]string{"parent"}).AddRow("gone").AddRow("also-gone"))

	agent.cleanupDeletedNamespaces(agent.snapshotConfig().NamespaceCleanup)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}

	messages := core.Messages()
	if len(messages) != 1 {
		t.Fatalf("got %d error logs, want 1: %v", len(messages), messages)
	}

	// The admin must be able to act on the message, so it has to report the
	// eligible namespaces and the setting that has to be raised.
	for _, want := range []string{"skipped", "maxNamespacesPerRun", "also-gone", "gone"} {
		if !strings.Contains(messages[0], want) {
			t.Errorf("error log %q does not contain %q", messages[0], want)
		}
	}
}

func TestFormatNamespaces(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		limit      int
		want       string
	}{
		{
			name:       "below the limit",
			namespaces: []string{"a", "b"},
			limit:      3,
			want:       "a, b",
		},
		{
			name:       "at the limit",
			namespaces: []string{"a", "b", "c"},
			limit:      3,
			want:       "a, b, c",
		},
		{
			name:       "above the limit is truncated",
			namespaces: []string{"a", "b", "c", "d", "e"},
			limit:      3,
			want:       "a, b, c and 2 more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatNamespaces(tt.namespaces, tt.limit); got != tt.want {
				t.Errorf("formatNamespaces() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgent_snapshotConfig_isIndependentFromLaterUpdates(t *testing.T) {
	cleanup := defaultCleanupConfig()
	cleanup.ExcludeNamespaces = []string{"kept"}

	agent, _, closeDB := newTestAgent(t, cleanup, &fakeNamespaceLister{})
	defer closeDB()

	snapshot := agent.snapshotConfig()

	// A ConfigMap update landing after the snapshot was taken must not alter it.
	agent.setAgentConfig(&config.RetentionPolicy{
		RunAt: "0 0 * * *",
		NamespaceCleanup: config.NamespaceCleanup{
			Enabled:             false,
			DryRun:              true,
			ExcludeNamespaces:   []string{"other"},
			MaxNamespacesPerRun: 1,
		},
	})

	if !snapshot.NamespaceCleanup.Enabled {
		t.Error("snapshot.NamespaceCleanup.Enabled = false, want true")
	}
	if snapshot.NamespaceCleanup.DryRun {
		t.Error("snapshot.NamespaceCleanup.DryRun = true, want false")
	}
	if got := snapshot.NamespaceCleanup.MaxNamespacesPerRun; got != cleanup.MaxNamespacesPerRun {
		t.Errorf("snapshot.NamespaceCleanup.MaxNamespacesPerRun = %d, want %d", got, cleanup.MaxNamespacesPerRun)
	}
	if got := snapshot.NamespaceCleanup.ExcludeNamespaces; len(got) != 1 || got[0] != "kept" {
		t.Errorf("snapshot.NamespaceCleanup.ExcludeNamespaces = %v, want [kept]", got)
	}

	// The snapshot must own its slices, so mutating it cannot corrupt the
	// configuration of the agent.
	snapshot.NamespaceCleanup.ExcludeNamespaces[0] = "mutated"
	if got := agent.snapshotConfig().NamespaceCleanup.ExcludeNamespaces; len(got) != 1 || got[0] != "other" {
		t.Errorf("agent configuration was altered through the snapshot: %v", got)
	}
}

func TestAgent_deleteNamespace_rejectsEmptyNamespace(t *testing.T) {
	agent, mock, closeDB := newTestAgent(t, defaultCleanupConfig(), &fakeNamespaceLister{})
	defer closeDB()

	if _, err := agent.deleteNamespace(""); err == nil {
		t.Error("deleteNamespace(\"\") = nil error, want error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestKubeNamespaceLister_List(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tekton-pipelines"}},
	)

	lister := &kubeNamespaceLister{client: client}
	got, err := lister.List(context.Background())
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}

	want := map[string]bool{"default": true, "tekton-pipelines": true}
	if len(got) != len(want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
	for _, ns := range got {
		if !want[ns] {
			t.Errorf("List() returned unexpected namespace %q", ns)
		}
	}
}
