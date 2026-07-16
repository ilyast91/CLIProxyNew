//go:build !race

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const (
	slaRequestCount = 200
	slaWorkers      = 4
	slaTargetRatio  = 0.95
)

func TestIntegrationRuntimeSLA(t *testing.T) {
	if testing.Short() {
		t.Skip("integration SLA requires Docker")
	}

	spans := installSLASpanRecorder(t)
	harness := newRuntimeHarness(t)
	apiKey := createSLAAPIKey(t, harness)
	if err := executeSLAInference(harness, apiKey); err != nil {
		t.Fatalf("warm up inference: %v", err)
	}

	jobs := make(chan struct{})
	errors := make(chan error, slaRequestCount)
	var workers sync.WaitGroup
	for range slaWorkers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range jobs {
				if err := executeSLAInference(harness, apiKey); err != nil {
					errors <- err
				}
			}
		}()
	}
	for range slaRequestCount {
		jobs <- struct{}{}
	}
	close(jobs)
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}

	metricsBody := scrapeSLAMetrics(t, harness)
	hits := requireSLAMetric(t, metricsBody, "cliproxy_cache_lookups_total", map[string]string{
		"cache": "api_key_auth", "outcome": "hit",
	})
	misses := requireSLAMetric(t, metricsBody, "cliproxy_cache_lookups_total", map[string]string{
		"cache": "api_key_auth", "outcome": "miss",
	})

	accessP95 := slaSpanP95(t, spans.Ended(), "access.Provider.Authenticate")
	selectorP95 := slaSpanP95(t, spans.Ended(), "selector.Pick")
	businessP95 := accessP95 + selectorP95
	cacheRatio := hits / (hits + misses)
	if businessP95 > 5*time.Millisecond {
		t.Fatalf("business overhead p95 = %s, want <= 5ms (access=%s selector=%s)", businessP95, accessP95, selectorP95)
	}
	if cacheRatio < slaTargetRatio {
		t.Fatalf("API-key cache hit ratio = %.4f, want >= %.2f (hits=%.0f misses=%.0f)", cacheRatio, slaTargetRatio, hits, misses)
	}
	t.Logf("SLA passed: business_p95=%s access_p95=%s selector_p95=%s cache_hit=%.4f requests=%d", businessP95, accessP95, selectorP95, cacheRatio, slaRequestCount)
}

func createSLAAPIKey(t *testing.T, harness *runtimeHarness) string {
	t.Helper()
	var login struct {
		UserID int64 `json:"user_id"`
	}
	if status := harness.requestJSON(t, http.MethodPost, "/api/v1/login", "", map[string]string{
		"username": "admin", "password": "admin-password",
	}, &login); status != http.StatusOK || login.UserID <= 0 {
		t.Fatalf("SLA login status=%d user_id=%d", status, login.UserID)
	}
	var created struct {
		Key string `json:"key"`
	}
	if status := harness.requestJSON(t, http.MethodPost, "/api/v1/me/keys", "", map[string]any{
		"name": "sla-gate", "scope": map[string]any{"inference": true},
	}, &created); status != http.StatusCreated || created.Key == "" {
		t.Fatalf("SLA API-key status=%d key_empty=%t", status, created.Key == "")
	}
	return created.Key
}

func executeSLAInference(harness *runtimeHarness, apiKey string) error {
	body := []byte(`{"model":"claude-sonnet-4-5-20250929","messages":[{"role":"user","content":"ping"}],"stream":false}`)
	request, err := http.NewRequest(http.MethodPost, harness.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create inference request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := harness.client.Do(request)
	if err != nil {
		return fmt.Errorf("execute inference request: %w", err)
	}
	defer response.Body.Close()
	_, readErr := io.Copy(io.Discard, response.Body)
	if readErr != nil {
		return fmt.Errorf("read inference response: %w", readErr)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("inference status=%d", response.StatusCode)
	}
	return nil
}

func scrapeSLAMetrics(t *testing.T, harness *runtimeHarness) string {
	t.Helper()
	response := httptest.NewRecorder()
	harness.metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%q", response.Code, response.Body.String())
	}
	return response.Body.String()
}

func requireSLAMetric(t *testing.T, body, name string, labels map[string]string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, name+"{") {
			continue
		}
		matches := true
		for key, value := range labels {
			if !strings.Contains(line, key+"=\""+value+"\"") {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid Prometheus metric line %q", line)
		}
		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("parse Prometheus metric line %q: %v", line, err)
		}
		return value
	}
	t.Fatalf("metric %s with labels %v not found", name, labels)
	return 0
}

func installSLASpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return recorder
}

func slaSpanP95(t *testing.T, spans []trace.ReadOnlySpan, name string) time.Duration {
	t.Helper()
	durations := make([]time.Duration, 0, slaRequestCount+1)
	for _, span := range spans {
		if span.Name() == name {
			durations = append(durations, span.EndTime().Sub(span.StartTime()))
		}
	}
	if len(durations) < slaRequestCount {
		t.Fatalf("ended spans %q = %d, want at least %d", name, len(durations), slaRequestCount)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	index := (95*len(durations) + 99) / 100
	return durations[index-1]
}
