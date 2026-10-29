//go:build telemetry && outage

package telemetry

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Run explicitly after stopping only amp-telemetry's collector. The normal
// telemetry-check target does not alter service availability.
func TestCollectorProcessOutage(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	started := time.Now()
	for _, base := range []string{"http://127.0.0.1:19595", "http://127.0.0.1:19596"} {
		for range 50 {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/api/v2/status", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"version":"0.34.1"`) {
				t.Fatalf("collector outage interrupted upstream request: %d %s, %v", resp.StatusCode, body, err)
			}
		}
	}
	t.Logf("100 real upstream responses during collector outage in %s", time.Since(started))
}
