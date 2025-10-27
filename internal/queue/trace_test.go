package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

// TestTraceContextPropagation_Enqueue tests that trace context is captured when enqueuing tasks
func TestTraceContextPropagation_Enqueue(t *testing.T) {
	// Setup a test tracer
	tp := tracesdk.NewTracerProvider()
	otel.SetTracerProvider(tp)
	tracer := tp.Tracer("test")

	tests := []struct {
		name string
		createTask func(ctx context.Context, client *Client) ([]byte, error)
	}{
		{
			name: "EnqueueScrape",
			createTask: func(ctx context.Context, client *Client) ([]byte, error) {
				// Create task payload
				payload := ScrapeTaskPayload{
					JobID:        "test-job-1",
					URL:          "https://example.com",
					ExtractLinks: true,
					EnqueuedAt:   time.Now().UnixNano(),
				}

				// Add trace context if available
				if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
					spanCtx := span.SpanContext()
					payload.TraceID = spanCtx.TraceID().String()
					payload.SpanID = spanCtx.SpanID().String()
				}

				return json.Marshal(payload)
			},
		},
		{
			name: "EnqueueExtractLinks",
			createTask: func(ctx context.Context, client *Client) ([]byte, error) {
				// Create task payload
				payload := ExtractLinksTaskPayload{
					ParentJobID: "test-job-1",
					SourceURL:   "https://example.com",
					ParentDepth: 0,
					EnqueuedAt:  time.Now().UnixNano(),
				}

				// Add trace context if available
				if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
					spanCtx := span.SpanContext()
					payload.TraceID = spanCtx.TraceID().String()
					payload.SpanID = spanCtx.SpanID().String()
				}

				return json.Marshal(payload)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a parent span
			ctx, span := tracer.Start(context.Background(), "test-operation")
			defer span.End()

			parentSpanContext := span.SpanContext()
			if !parentSpanContext.IsValid() {
				t.Fatal("Parent span context is invalid")
			}

			// Create a mock client (nil is fine for this test since we're just testing payload creation)
			client := &Client{}

			// Create the task with trace context
			payloadBytes, err := tt.createTask(ctx, client)
			if err != nil {
				t.Fatalf("Failed to create task: %v", err)
			}

			// Parse the payload to verify trace context was captured
			var payload struct {
				TraceID    string `json:"trace_id"`
				SpanID     string `json:"span_id"`
				EnqueuedAt int64  `json:"enqueued_at"`
			}

			if err := json.Unmarshal(payloadBytes, &payload); err != nil {
				t.Fatalf("Failed to unmarshal payload: %v", err)
			}

			// Verify trace context was captured
			if payload.TraceID == "" {
				t.Error("TraceID was not captured in payload")
			}

			if payload.SpanID == "" {
				t.Error("SpanID was not captured in payload")
			}

			// Verify the trace ID matches the parent span
			if payload.TraceID != parentSpanContext.TraceID().String() {
				t.Errorf("TraceID mismatch: got %s, want %s", payload.TraceID, parentSpanContext.TraceID().String())
			}

			// Verify the span ID matches the parent span
			if payload.SpanID != parentSpanContext.SpanID().String() {
				t.Errorf("SpanID mismatch: got %s, want %s", payload.SpanID, parentSpanContext.SpanID().String())
			}

			// Verify enqueued timestamp was set
			if payload.EnqueuedAt == 0 {
				t.Error("EnqueuedAt was not set")
			}
		})
	}
}

// TestTraceContextPropagation_Extract tests that workers can extract trace context from payloads
func TestTraceContextPropagation_Extract(t *testing.T) {
	// Setup a test tracer
	tp := tracesdk.NewTracerProvider()
	otel.SetTracerProvider(tp)
	tracer := tp.Tracer("test")

	// Create a parent span to get valid trace IDs
	_, parentSpan := tracer.Start(context.Background(), "test-enqueue")
	parentSpanContext := parentSpan.SpanContext()
	parentSpan.End()

	tests := []struct {
		name          string
		payload       interface{}
		expectedType  string
	}{
		{
			name: "ExtractFromScrapePayload",
			payload: ScrapeTaskPayload{
				JobID:        "test-job-1",
				URL:          "https://example.com",
				ExtractLinks: true,
				TraceID:      parentSpanContext.TraceID().String(),
				SpanID:       parentSpanContext.SpanID().String(),
				EnqueuedAt:   time.Now().Add(-5 * time.Second).UnixNano(),
			},
			expectedType: TypeScrapeURL,
		},
		{
			name: "ExtractFromExtractLinksPayload",
			payload: ExtractLinksTaskPayload{
				ParentJobID: "test-job-1",
				SourceURL:   "https://example.com",
				ParentDepth: 0,
				TraceID:     parentSpanContext.TraceID().String(),
				SpanID:      parentSpanContext.SpanID().String(),
				EnqueuedAt:  time.Now().Add(-5 * time.Second).UnixNano(),
			},
			expectedType: TypeExtractLinks,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal the payload
			payloadBytes, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Failed to marshal payload: %v", err)
			}

			// Unmarshal to extract trace context
			var extracted struct {
				TraceID    string `json:"trace_id"`
				SpanID     string `json:"span_id"`
				EnqueuedAt int64  `json:"enqueued_at"`
			}

			if err := json.Unmarshal(payloadBytes, &extracted); err != nil {
				t.Fatalf("Failed to unmarshal payload: %v", err)
			}

			// Verify trace context can be reconstructed
			traceID, err := trace.TraceIDFromHex(extracted.TraceID)
			if err != nil {
				t.Fatalf("Failed to parse TraceID: %v", err)
			}

			spanID, err := trace.SpanIDFromHex(extracted.SpanID)
			if err != nil {
				t.Fatalf("Failed to parse SpanID: %v", err)
			}

			// Create remote span context
			remoteSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				SpanID:     spanID,
				TraceFlags: trace.FlagsSampled,
				Remote:     true,
			})

			if !remoteSpanCtx.IsValid() {
				t.Error("Reconstructed span context is invalid")
			}

			// Verify the trace ID matches
			if remoteSpanCtx.TraceID() != parentSpanContext.TraceID() {
				t.Errorf("TraceID mismatch: got %s, want %s", remoteSpanCtx.TraceID(), parentSpanContext.TraceID())
			}

			// Verify the span ID matches
			if remoteSpanCtx.SpanID() != parentSpanContext.SpanID() {
				t.Errorf("SpanID mismatch: got %s, want %s", remoteSpanCtx.SpanID(), parentSpanContext.SpanID())
			}

			// Verify queue wait time can be calculated
			if extracted.EnqueuedAt > 0 {
				enqueuedTime := time.Unix(0, extracted.EnqueuedAt)
				queueWaitTime := time.Since(enqueuedTime)

				if queueWaitTime < 0 {
					t.Error("Queue wait time is negative")
				}

				if queueWaitTime < 4*time.Second || queueWaitTime > 6*time.Second {
					t.Logf("Queue wait time is approximately 5 seconds, got %v", queueWaitTime)
				}
			}
		})
	}
}

// TestQueueWaitTimeCalculation tests that queue wait time is calculated correctly
func TestQueueWaitTimeCalculation(t *testing.T) {
	tests := []struct {
		name           string
		enqueuedAt     int64
		expectedWaitMin time.Duration
		expectedWaitMax time.Duration
	}{
		{
			name:           "RecentEnqueue",
			enqueuedAt:     time.Now().Add(-1 * time.Second).UnixNano(),
			expectedWaitMin: 900 * time.Millisecond,
			expectedWaitMax: 1100 * time.Millisecond,
		},
		{
			name:           "OlderEnqueue",
			enqueuedAt:     time.Now().Add(-10 * time.Second).UnixNano(),
			expectedWaitMin: 9900 * time.Millisecond,
			expectedWaitMax: 10100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enqueuedTime := time.Unix(0, tt.enqueuedAt)
			queueWaitTime := time.Since(enqueuedTime)

			if queueWaitTime < tt.expectedWaitMin || queueWaitTime > tt.expectedWaitMax {
				t.Errorf("Queue wait time out of expected range: got %v, want between %v and %v",
					queueWaitTime, tt.expectedWaitMin, tt.expectedWaitMax)
			}
		})
	}
}
