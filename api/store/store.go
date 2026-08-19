// Package store exposes the generated database query types for use by
// other modules in the workspace (e.g. the worker module).
package store

import (
	"database/sql"

	apidb "github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
)

// Open opens the SQLite database at path and runs all pending migrations.
func Open(path string) (*sql.DB, error) {
	return apidb.Open(path)
}

// ---------------------------------------------------------------------------
// Core infrastructure
// ---------------------------------------------------------------------------

// DBTX is the public alias for the generated database/transaction interface.
type DBTX = generated.DBTX

// Queries is the public alias for the generated sqlc query struct.
type Queries = generated.Queries

// New creates a new Queries instance backed by the given database connection.
func New(db DBTX) *Queries {
	return generated.New(db)
}

// ---------------------------------------------------------------------------
// Model types (from models.go)
// ---------------------------------------------------------------------------

// CheckResult is the public alias for the generated check_results row type.
type CheckResult = generated.CheckResult

// Incident is the public alias for the generated incidents row type.
type Incident = generated.Incident

// IncidentUpdate is the public alias for the generated incident_updates row type.
type IncidentUpdate = generated.IncidentUpdate

// InfraAgent is the public alias for the generated infra_agents row type.
type InfraAgent = generated.InfraAgent

// InfraMetrics1m is the public alias for the generated infra_metrics_1m row type.
type InfraMetrics1m = generated.InfraMetrics1m

// InfraMetricsRaw is the public alias for the generated infra_metrics_raw row type.
type InfraMetricsRaw = generated.InfraMetricsRaw

// ImportRun is the public alias for the generated import_runs row type.
type ImportRun = generated.ImportRun

// Monitor is the public alias for the generated monitors row type.
type Monitor = generated.Monitor

// MonitorGroup is the public alias for the generated monitor_groups row type.
type MonitorGroup = generated.MonitorGroup

// Notification is the public alias for the generated notifications row type.
type Notification = generated.Notification

// PushMonitorToken is the public alias for the generated push_monitor_tokens row type.
type PushMonitorToken = generated.PushMonitorToken

// SslCheck is the public alias for the generated ssl_checks row type.
type SslCheck = generated.SslCheck

// ThemeConfig is the public alias for the generated theme_config row type.
type ThemeConfig = generated.ThemeConfig

// ---------------------------------------------------------------------------
// Param types — agents
// ---------------------------------------------------------------------------

// CreateAgentParams is the public alias for the generated parameter type.
type CreateAgentParams = generated.CreateAgentParams

// ---------------------------------------------------------------------------
// Param types — diagnostics
// ---------------------------------------------------------------------------

// InsertIncidentDiagnosticParams is the public alias for the generated parameter type.
type InsertIncidentDiagnosticParams = generated.InsertIncidentDiagnosticParams

// ListAgentDiagnosticsParams is the public alias for the generated parameter type.
type ListAgentDiagnosticsParams = generated.ListAgentDiagnosticsParams

// ---------------------------------------------------------------------------
// Param types — check_results
// ---------------------------------------------------------------------------

// InsertCheckResultParams is the public alias for the generated parameter type.
type InsertCheckResultParams = generated.InsertCheckResultParams

// ListCheckResultsParams is the public alias for the generated parameter type.
type ListCheckResultsParams = generated.ListCheckResultsParams

// UptimePercentParams is the public alias for the generated parameter type.
type UptimePercentParams = generated.UptimePercentParams

// ---------------------------------------------------------------------------
// Param types — groups
// ---------------------------------------------------------------------------

// CreateGroupParams is the public alias for the generated parameter type.
type CreateGroupParams = generated.CreateGroupParams

// CreateImportedGroupParams is the public alias for the generated parameter type.
type CreateImportedGroupParams = generated.CreateImportedGroupParams

// GetGroupBySourceExternalIDParams is the public alias for the generated parameter type.
type GetGroupBySourceExternalIDParams = generated.GetGroupBySourceExternalIDParams

// UpdateGroupParams is the public alias for the generated parameter type.
type UpdateGroupParams = generated.UpdateGroupParams

// UpdateImportedGroupParams is the public alias for the generated parameter type.
type UpdateImportedGroupParams = generated.UpdateImportedGroupParams

// ---------------------------------------------------------------------------
// Param types — imports
// ---------------------------------------------------------------------------

// CompleteImportRunParams is the public alias for the generated parameter type.
type CompleteImportRunParams = generated.CompleteImportRunParams

// CreateImportRunParams is the public alias for the generated parameter type.
type CreateImportRunParams = generated.CreateImportRunParams

// FailImportRunParams is the public alias for the generated parameter type.
type FailImportRunParams = generated.FailImportRunParams

// ---------------------------------------------------------------------------
// Param types — incident_updates
// ---------------------------------------------------------------------------

// CreateIncidentUpdateParams is the public alias for the generated parameter type.
type CreateIncidentUpdateParams = generated.CreateIncidentUpdateParams

// ---------------------------------------------------------------------------
// Param types — incidents
// ---------------------------------------------------------------------------

// CreateIncidentParams is the public alias for the generated parameter type.
type CreateIncidentParams = generated.CreateIncidentParams

// CreateAutoIncidentParams is the parameter type for atomic monitor incidents.
type CreateAutoIncidentParams = generated.CreateAutoIncidentParams

// UpdateIncidentRCAParams is the public alias for the generated parameter type.
type UpdateIncidentRCAParams = generated.UpdateIncidentRCAParams

// UpdateIncidentStatusParams is the public alias for the generated parameter type.
type UpdateIncidentStatusParams = generated.UpdateIncidentStatusParams

// ---------------------------------------------------------------------------
// Param types — infra_metrics
// ---------------------------------------------------------------------------

// InsertRawMetricParams is the public alias for the generated parameter type.
type InsertRawMetricParams = generated.InsertRawMetricParams

// List1mMetricsParams is the public alias for the generated parameter type.
type List1mMetricsParams = generated.List1mMetricsParams

// ListRawMetricsParams is the public alias for the generated parameter type.
type ListRawMetricsParams = generated.ListRawMetricsParams

// Upsert1mMetricParams is the public alias for the generated parameter type.
type Upsert1mMetricParams = generated.Upsert1mMetricParams

// ---------------------------------------------------------------------------
// Param types — monitors
// ---------------------------------------------------------------------------

// CreateMonitorParams is the public alias for the generated parameter type.
type CreateMonitorParams = generated.CreateMonitorParams

// GetMonitorBySourceExternalIDParams is the public alias for the generated parameter type.
type GetMonitorBySourceExternalIDParams = generated.GetMonitorBySourceExternalIDParams

// UpdateMonitorParams is the public alias for the generated parameter type.
type UpdateMonitorParams = generated.UpdateMonitorParams

// ---------------------------------------------------------------------------
// Param types — notifications
// ---------------------------------------------------------------------------

// CreateNotificationParams is the public alias for the generated parameter type.
type CreateNotificationParams = generated.CreateNotificationParams

// UpdateNotificationParams is the public alias for the generated parameter type.
type UpdateNotificationParams = generated.UpdateNotificationParams

// ---------------------------------------------------------------------------
// Param types — push tokens
// ---------------------------------------------------------------------------

// UpsertPushTokenParams is the public alias for the generated parameter type.
type UpsertPushTokenParams = generated.UpsertPushTokenParams

// ---------------------------------------------------------------------------
// Param types — theme
// ---------------------------------------------------------------------------

// UpsertThemeParams is the public alias for the generated parameter type.
type UpsertThemeParams = generated.UpsertThemeParams

// ---------------------------------------------------------------------------
// Model types — maintenance
// ---------------------------------------------------------------------------

// MaintenanceWindow is the public alias for the generated maintenance_windows row type.
type MaintenanceWindow = generated.MaintenanceWindow

// ---------------------------------------------------------------------------
// Param types — maintenance
// ---------------------------------------------------------------------------

// CreateMaintenanceParams is the public alias for the generated parameter type.
type CreateMaintenanceParams = generated.CreateMaintenanceParams

// UpdateMaintenanceStatusParams is the public alias for the generated parameter type.
type UpdateMaintenanceStatusParams = generated.UpdateMaintenanceStatusParams
