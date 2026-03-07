package domain

import "time"

const (
	WindowDay  = "day"
	WindowWeek = "week"
)

const (
	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
)

type ReportDefinition struct {
	Slug             string            `json:"slug"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	CacheTTLSeconds  int               `json:"cache_ttl_seconds"`
	DefaultWindow    string            `json:"default_window"`
	AllowedWindows   []string          `json:"allowed_windows"`
	SupportedFilters map[string]string `json:"supported_filters"`
}

type AggregateWindow struct {
	ReportSlug             string `json:"report_slug"`
	Window                 string `json:"window"`
	RetentionDays          int    `json:"retention_days"`
	RefreshIntervalMinutes int    `json:"refresh_interval_minutes"`
	IsDefault              bool   `json:"is_default"`
}

type ReportListFilter struct {
	Search string
	Limit  int
	Offset int
	Sort   string
	Order  string
}

type ReportRunParams struct {
	Window    string
	DateFrom  time.Time
	DateTo    time.Time
	Breakdown string
	Limit     int
	Offset    int
	Source    string
	Status    string
}

type ReportResult struct {
	ReportSlug   string           `json:"report_slug"`
	Window       string           `json:"window"`
	DateFrom     string           `json:"date_from"`
	DateTo       string           `json:"date_to"`
	GeneratedAt  time.Time        `json:"generated_at"`
	Rows         []map[string]any `json:"rows"`
	CacheHit     bool             `json:"cache_hit"`
	RowCount     int              `json:"row_count"`
	ExecutionMS  int64            `json:"execution_ms"`
	SourceSystem string           `json:"source_system"`
}

type RecomputeRequest struct {
	ReportSlug   string
	Window       string
	DateFrom     time.Time
	DateTo       time.Time
	RequestedBy  string
	RequestedVia string
}

type RecomputeRun struct {
	ID           string         `json:"id"`
	ReportSlug   string         `json:"report_slug"`
	Window       string         `json:"window"`
	DateFrom     string         `json:"date_from"`
	DateTo       string         `json:"date_to"`
	RequestedBy  string         `json:"requested_by"`
	Status       string         `json:"status"`
	RequestedAt  time.Time      `json:"requested_at"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Summary      map[string]any `json:"summary"`
}

type RecomputeSummary struct {
	RowsDeleted  int64 `json:"rows_deleted"`
	RowsInserted int64 `json:"rows_inserted"`
	BucketCount  int64 `json:"bucket_count"`
}

type AuditEntry struct {
	ID           int64          `json:"id"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}

type AuditFilter struct {
	Action string
	Actor  string
	Limit  int
	Offset int
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type HealthStatus struct {
	Status       string            `json:"status"`
	Service      string            `json:"service"`
	Version      string            `json:"version"`
	Timestamp    time.Time         `json:"timestamp"`
	Dependencies map[string]string `json:"dependencies"`
}
