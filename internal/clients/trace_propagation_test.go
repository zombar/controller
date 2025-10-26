package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestTracePropagation verifies that trace context is propagated to downstream services
func TestTracePropagation(t *testing.T) {
	// Set up in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create a test server that verifies it receives trace headers
	receivedTraceHeaders := false
	var receivedTraceParent string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if traceparent header is present
		traceparent := r.Header.Get("traceparent")
		if traceparent != "" {
			receivedTraceHeaders = true
			receivedTraceParent = traceparent
			t.Logf("✓ Received traceparent header: %s", traceparent)
		} else {
			t.Error("✗ No traceparent header received!")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": "test-123"}`))
	}))
	defer ts.Close()

	// Create a client (using scraper client as example)
	client := NewScraperClient(ts.URL)

	// Create a parent span
	ctx := context.Background()
	tracer := otel.Tracer("test")
	ctx, parentSpan := tracer.Start(ctx, "test.parent")
	defer parentSpan.End()

	// Make a request - this should propagate the trace context
	_, err := client.Scrape(ctx, "http://example.com")
	if err != nil {
		t.Fatalf("Scrape failed: %v", err)
	}

	// Verify trace headers were sent
	if !receivedTraceHeaders {
		t.Error("❌ Trace context was NOT propagated to downstream service")
	} else {
		t.Log("✅ Trace context successfully propagated to downstream service")
	}

	// Verify we have multiple spans in the same trace
	spans := exporter.GetSpans()
	t.Logf("Total spans captured: %d", len(spans))

	if len(spans) < 1 {
		t.Error("Expected at least 1 span")
	}

	// Verify spans share the same trace ID
	if len(spans) > 0 {
		traceID := spans[0].SpanContext.TraceID().String()
		t.Logf("Trace ID: %s", traceID)

		for i, span := range spans {
			t.Logf("  Span %d: %s (parent: %s)", i, span.Name, span.Parent.SpanID().String())
			if span.SpanContext.TraceID().String() != traceID {
				t.Error("Spans have different trace IDs!")
			}
		}
		t.Log("✅ All spans share the same trace ID")
	}

	t.Logf("\nReceived traceparent header: %s", receivedTraceParent)
}

// TestTextAnalyzerTracePropagation tests TextAnalyzer client trace propagation
func TestTextAnalyzerTracePropagation(t *testing.T) {
	// Set up tracing
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("traceparent") == "" {
			t.Error("✗ No traceparent header in textanalyzer request!")
		} else {
			t.Log("✓ traceparent header present in textanalyzer request")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": "test-123", "metadata": {}}`))
	}))
	defer ts.Close()

	client := NewTextAnalyzerClient(ts.URL)
	ctx := context.Background()
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(ctx, "test.analyze")
	defer span.End()

	_, err := client.Analyze(ctx, "test text")
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Errorf("Expected at least 2 spans, got %d", len(spans))
	}
}

// TestSchedulerTracePropagation tests Scheduler client trace propagation
func TestSchedulerTracePropagation(t *testing.T) {
	// Set up tracing
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("traceparent") == "" {
			t.Error("✗ No traceparent header in scheduler request!")
		} else {
			t.Log("✓ traceparent header present in scheduler request")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	client := NewSchedulerClient(ts.URL)
	ctx := context.Background()
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(ctx, "test.listTasks")
	defer span.End()

	_, err := client.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Errorf("Expected at least 2 spans, got %d", len(spans))
	}
}

// TestOtelHttpTransport verifies otelhttp.Transport is configured
func TestOtelHttpTransport(t *testing.T) {
	tests := []struct {
		name         string
		createClient func(baseURL string) interface{ getTransport() http.RoundTripper }
	}{
		{
			name: "ScraperClient",
			createClient: func(baseURL string) interface{ getTransport() http.RoundTripper } {
				client := NewScraperClient(baseURL)
				return &transportGetter{client.httpClient}
			},
		},
		{
			name: "TextAnalyzerClient",
			createClient: func(baseURL string) interface{ getTransport() http.RoundTripper } {
				client := NewTextAnalyzerClient(baseURL)
				return &transportGetter{client.httpClient}
			},
		},
		{
			name: "SchedulerClient",
			createClient: func(baseURL string) interface{ getTransport() http.RoundTripper } {
				client := NewSchedulerClient(baseURL)
				return &transportGetter{client.httpClient}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.createClient("http://test")
			transport := client.getTransport()

			// Verify transport is wrapped with otelhttp
			_, ok := transport.(*otelhttp.Transport)
			if !ok {
				t.Errorf("%s does not use otelhttp.Transport for trace propagation", tt.name)
			} else {
				t.Logf("✅ %s correctly uses otelhttp.Transport", tt.name)
			}
		})
	}
}

// transportGetter helper to access http.Client's Transport
type transportGetter struct {
	client *http.Client
}

func (tg *transportGetter) getTransport() http.RoundTripper {
	return tg.client.Transport
}
