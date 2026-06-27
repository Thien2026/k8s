package harbor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArtifactScanOverview_Harbor215(t *testing.T) {
	body := `{
		"scan_overview": {
			"application/vnd.security.vulnerability.report; version=1.1": {
				"scan_status": "Success",
				"severity": "Critical",
				"summary": {
					"fixable": 36,
					"total": 36,
					"summary": {"Critical": 1, "High": 15, "Low": 2, "Medium": 18}
				}
			}
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "admin", "pass")
	ov, err := c.ArtifactScanOverview(context.Background(), "research-labs", "app", "e4107718")
	if err != nil {
		t.Fatal(err)
	}
	if ov.Status != "success" {
		t.Fatalf("status=%q want success", ov.Status)
	}
	if ov.Total != 36 {
		t.Fatalf("total=%d want 36", ov.Total)
	}
	if ov.Severity["Critical"] != 1 {
		t.Fatalf("critical=%d", ov.Severity["Critical"])
	}
	if ov.Detail == "" {
		t.Fatal("expected detail")
	}
}
