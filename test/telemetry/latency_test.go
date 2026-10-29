//go:build telemetry

package telemetry

import (
	"io"
	"net/http"
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestPassThroughP99Overhead(t *testing.T) {
	client := &http.Client{Timeout: 3 * time.Second}
	measure := func(address string) time.Duration {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address+"/api/v2/receivers", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("User-Agent", "AmpBench/1")
		started := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("benchmark request failed: %d %v", resp.StatusCode, err)
		}
		return time.Since(started)
	}
	const off, on = "http://127.0.0.1:19597", "http://127.0.0.1:19595"
	for range 100 {
		measure(off)
		measure(on)
	}
	disabled, enabled := make([]time.Duration, 1000), make([]time.Duration, 1000)
	for i := range disabled {
		// Interleave to reduce ordering and host-load bias; connections stay warm.
		disabled[i], enabled[i] = measure(off), measure(on)
	}
	slices.Sort(disabled)
	slices.Sort(enabled)
	p99off, p99on := disabled[989], enabled[989]
	t.Logf("samples_per_mode=1000 warmup_per_mode=100 concurrency=1 go=%s os=%s arch=%s cpus=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	t.Logf("p99_off_ms=%.3f p99_on_ms=%.3f added_p99_ms=%.3f (full HTTP response, telemetry includes JSON access logs and real asynchronous OTLP export)",
		float64(p99off)/float64(time.Millisecond), float64(p99on)/float64(time.Millisecond), float64(p99on-p99off)/float64(time.Millisecond))
}
