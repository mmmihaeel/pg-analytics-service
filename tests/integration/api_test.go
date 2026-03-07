package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pg-analytics-service/pg-analytics-service/tests/testutil"
)

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error *apiErr         `json:"error"`
}

type apiErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func TestHealthEndpoint(t *testing.T) {
	harness := testutil.NewServerHarness(t, true)
	defer harness.Close()

	status, body := doRequest(t, http.MethodGet, harness.Server.URL+"/api/v1/health", "", nil)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	if body.Error != nil {
		t.Fatalf("expected no error in response, got %#v", body.Error)
	}
}

func TestReportListingAndDetail(t *testing.T) {
	harness := testutil.NewServerHarness(t, true)
	defer harness.Close()

	status, body := doRequest(t, http.MethodGet, harness.Server.URL+"/api/v1/reports?limit=2&offset=0", "", nil)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	var reports []map[string]any
	mustUnmarshal(t, body.Data, &reports)
	if len(reports) == 0 {
		t.Fatal("expected at least one report definition")
	}

	status, body = doRequest(t, http.MethodGet, harness.Server.URL+"/api/v1/reports/volume-by-period", "", nil)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	var report map[string]any
	mustUnmarshal(t, body.Data, &report)
	if report["slug"] != "volume-by-period" {
		t.Fatalf("expected volume-by-period slug, got %v", report["slug"])
	}
}

func TestReportRunValidation(t *testing.T) {
	harness := testutil.NewServerHarness(t, true)
	defer harness.Close()

	status, body := doRequest(t, http.MethodGet, harness.Server.URL+"/api/v1/reports/volume-by-period/run?window=month", "", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid window, got %d", status)
	}
	if body.Error == nil || body.Error.Code != "invalid_request" {
		t.Fatalf("expected invalid_request error, got %#v", body.Error)
	}

	status, body = doRequest(t, http.MethodGet, harness.Server.URL+"/api/v1/reports/volume-by-period/run?date_from=2026-02-10&date_to=2026-01-01", "", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid date range, got %d", status)
	}
}

func TestReportCachingBehavior(t *testing.T) {
	harness := testutil.NewServerHarness(t, true)
	defer harness.Close()

	path := harness.Server.URL + "/api/v1/reports/status-counts/run?window=day&breakdown=status&date_from=2026-01-01&date_to=2026-02-01"
	status, body := doRequest(t, http.MethodGet, path, "", nil)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	var first map[string]any
	mustUnmarshal(t, body.Data, &first)
	if cacheHit, _ := first["cache_hit"].(bool); cacheHit {
		t.Fatal("expected first report run to be uncached")
	}

	status, body = doRequest(t, http.MethodGet, path, "", nil)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	var second map[string]any
	mustUnmarshal(t, body.Data, &second)
	if cacheHit, _ := second["cache_hit"].(bool); !cacheHit {
		t.Fatal("expected second report run to be served from cache")
	}
}

func TestManagementAuthRequired(t *testing.T) {
	harness := testutil.NewServerHarness(t, true)
	defer harness.Close()

	req := map[string]any{"report_slug": "status-counts", "window": "day", "date_from": "2026-01-01", "date_to": "2026-02-01"}
	status, _ := doRequest(t, http.MethodPost, harness.Server.URL+"/api/v1/recomputations", "", req)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 without management key, got %d", status)
	}

	status, _ = doRequest(t, http.MethodGet, harness.Server.URL+"/api/v1/audit-entries", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 without management key, got %d", status)
	}
}

func TestRecomputeTriggerAndRunStatus(t *testing.T) {
	harness := testutil.NewServerHarness(t, true)
	defer harness.Close()

	req := map[string]any{
		"report_slug":  "volume-by-period",
		"window":       "day",
		"date_from":    "2026-01-01",
		"date_to":      "2026-02-01",
		"requested_by": "integration-test",
	}
	status, body := doRequest(t, http.MethodPost, harness.Server.URL+"/api/v1/recomputations", harness.Env.ManagementKey, req)
	if status != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", status)
	}

	var run map[string]any
	mustUnmarshal(t, body.Data, &run)
	runID, _ := run["id"].(string)
	if runID == "" {
		t.Fatal("expected recompute run id in response")
	}

	var finalRun map[string]any
	for i := 0; i < 30; i++ {
		status, body = doRequest(t, http.MethodGet, harness.Server.URL+"/api/v1/recomputations/"+runID, harness.Env.ManagementKey, nil)
		if status != http.StatusOK {
			t.Fatalf("expected 200 while polling run status, got %d", status)
		}
		mustUnmarshal(t, body.Data, &finalRun)
		if finalRun["status"] == "completed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if finalRun["status"] != "completed" {
		t.Fatalf("expected completed status, got %v", finalRun["status"])
	}

	summary, _ := finalRun["summary"].(map[string]any)
	if summary == nil {
		t.Fatal("expected summary in recompute run response")
	}
	if summary["rows_inserted"] == nil {
		t.Fatalf("expected rows_inserted in summary, got %#v", summary)
	}
}

func TestDuplicateRecomputeIsLocked(t *testing.T) {
	harness := testutil.NewServerHarness(t, false)
	defer harness.Close()

	req := map[string]any{"report_slug": "status-counts", "window": "day", "date_from": "2026-01-01", "date_to": "2026-02-01"}
	status, _ := doRequest(t, http.MethodPost, harness.Server.URL+"/api/v1/recomputations", harness.Env.ManagementKey, req)
	if status != http.StatusAccepted {
		t.Fatalf("expected first trigger to return 202, got %d", status)
	}

	status, body := doRequest(t, http.MethodPost, harness.Server.URL+"/api/v1/recomputations", harness.Env.ManagementKey, req)
	if status != http.StatusConflict {
		t.Fatalf("expected duplicate trigger to return 409, got %d", status)
	}
	if body.Error == nil || body.Error.Code != "conflict" {
		t.Fatalf("expected conflict error code, got %#v", body.Error)
	}
}

func TestAuditEntriesEndpoint(t *testing.T) {
	harness := testutil.NewServerHarness(t, false)
	defer harness.Close()

	req := map[string]any{"report_slug": "top-entities", "window": "day", "date_from": "2026-01-01", "date_to": "2026-02-01", "requested_by": "auditor"}
	status, _ := doRequest(t, http.MethodPost, harness.Server.URL+"/api/v1/recomputations", harness.Env.ManagementKey, req)
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 for recompute trigger, got %d", status)
	}

	status, body := doRequest(t, http.MethodGet, harness.Server.URL+"/api/v1/audit-entries?limit=10", harness.Env.ManagementKey, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for audit endpoint, got %d", status)
	}

	var entries []map[string]any
	mustUnmarshal(t, body.Data, &entries)
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry")
	}

	hasTriggeredEntry := false
	for _, entry := range entries {
		if entry["action"] == "recompute.triggered" {
			hasTriggeredEntry = true
			break
		}
	}
	if !hasTriggeredEntry {
		t.Fatalf("expected recompute.triggered audit entry, got %v", entries)
	}
}

func TestDatabaseIsolationBetweenHarnesses(t *testing.T) {
	first := testutil.NewServerHarness(t, false)
	defer first.Close()

	req := map[string]any{"report_slug": "status-counts", "window": "day", "date_from": "2026-01-01", "date_to": "2026-02-01"}
	status, _ := doRequest(t, http.MethodPost, first.Server.URL+"/api/v1/recomputations", first.Env.ManagementKey, req)
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 for first recompute trigger, got %d", status)
	}

	status, body := doRequest(t, http.MethodGet, first.Server.URL+"/api/v1/audit-entries?limit=10", first.Env.ManagementKey, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for first harness audit query, got %d", status)
	}
	var entries []map[string]any
	mustUnmarshal(t, body.Data, &entries)
	if len(entries) == 0 {
		t.Fatal("expected audit entries after recompute trigger")
	}

	second := testutil.NewServerHarness(t, false)
	defer second.Close()

	status, body = doRequest(t, http.MethodGet, second.Server.URL+"/api/v1/audit-entries?limit=10", second.Env.ManagementKey, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for second harness audit query, got %d", status)
	}
	mustUnmarshal(t, body.Data, &entries)
	if len(entries) != 0 {
		t.Fatalf("expected clean audit table for isolated test state, got %d entries", len(entries))
	}
}

func doRequest(t *testing.T, method, url, managementKey string, payload any) (int, envelope) {
	t.Helper()

	var bodyBytes []byte
	if payload != nil {
		var err error
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if managementKey != "" {
		req.Header.Set("X-Management-Key", managementKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var decoded envelope
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	return resp.StatusCode, decoded
}

func mustUnmarshal(t *testing.T, blob json.RawMessage, target any) {
	t.Helper()
	if len(blob) == 0 {
		t.Fatalf("expected JSON payload, got empty: %s", fmt.Sprintf("%T", target))
	}
	if err := json.Unmarshal(blob, target); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
}
