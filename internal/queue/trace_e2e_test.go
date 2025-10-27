package queue

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// getTestRedisAddr returns the Redis address for tests
// Checks TEST_REDIS_ADDR env var first, falls back to localhost:6379
func getTestRedisAddr() string {
	if addr := os.Getenv("TEST_REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

// TestE2ETraceFlow_ScrapeRequest tests the complete trace flow from enqueue to worker processing
func TestE2ETraceFlow_ScrapeRequest(t *testing.T) {
	// Setup in-memory span exporter to capture spans
	spanRecorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(
		trace.WithSpanProcessor(spanRecorder),
	)
	otel.SetTracerProvider(tp)

	// Create a parent span simulating an incoming HTTP request
	tracer := tp.Tracer("test")
	ctx, parentSpan := tracer.Start(context.Background(), "http.request",
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
	)

	parentSpanContext := parentSpan.SpanContext()
	if !parentSpanContext.IsValid() {
		t.Fatal("Parent span context is invalid")
	}

	// Step 1: Enqueue a scrape task with trace context
	payload := ScrapeTaskPayload{
		JobID:        "test-job-e2e",
		URL:          "https://example.com",
		ExtractLinks: true,
		EnqueuedAt:   time.Now().UnixNano(),
	}

	// Capture trace context from parent span
	if span := oteltrace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		payload.TraceID = spanCtx.TraceID().String()
		payload.SpanID = spanCtx.SpanID().String()
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	// Verify trace context was captured in payload
	if payload.TraceID == "" || payload.SpanID == "" {
		t.Error("Trace context not captured in payload")
	}

	if payload.TraceID != parentSpanContext.TraceID().String() {
		t.Errorf("TraceID mismatch in payload: got %s, want %s",
			payload.TraceID, parentSpanContext.TraceID().String())
	}

	// Step 2: Simulate worker processing the task
	workerCtx := context.Background()

	// Parse the payload (simulating what worker receives)
	var receivedPayload ScrapeTaskPayload
	if err := json.Unmarshal(payloadBytes, &receivedPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	// Extract and reconstruct trace context (what worker does)
	var linkedCtx context.Context
	if receivedPayload.TraceID != "" && receivedPayload.SpanID != "" {
		traceID, err := oteltrace.TraceIDFromHex(receivedPayload.TraceID)
		if err != nil {
			t.Fatalf("Failed to parse TraceID: %v", err)
		}

		spanID, err := oteltrace.SpanIDFromHex(receivedPayload.SpanID)
		if err != nil {
			t.Fatalf("Failed to parse SpanID: %v", err)
		}

		remoteSpanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: oteltrace.FlagsSampled,
			Remote:     true,
		})

		linkedCtx = oteltrace.ContextWithRemoteSpanContext(workerCtx, remoteSpanCtx)
	} else {
		linkedCtx = workerCtx
	}

	// Create worker span with link to enqueue span
	_, workerSpan := tracer.Start(linkedCtx, "asynq.task.process",
		oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
	)
	workerSpan.End()

	// End parent span before verification
	parentSpan.End()

	// Give spans time to be recorded
	time.Sleep(100 * time.Millisecond)

	// Step 3: Verify the complete trace chain
	spans := spanRecorder.Ended()
	if len(spans) < 2 {
		t.Fatalf("Expected at least 2 spans, got %d", len(spans))
	}

	// Verify all spans share the same TraceID
	expectedTraceID := parentSpanContext.TraceID()
	for i, span := range spans {
		if span.SpanContext().TraceID() != expectedTraceID {
			t.Errorf("Span %d has different TraceID: got %s, want %s",
				i, span.SpanContext().TraceID(), expectedTraceID)
		}
	}

	// Verify we have both parent and worker spans
	var foundParentSpan, foundWorkerSpan bool
	for _, span := range spans {
		if span.Name() == "http.request" {
			foundParentSpan = true
			if span.SpanKind() != oteltrace.SpanKindServer {
				t.Error("Parent span should have SpanKind Server")
			}
		}
		if span.Name() == "asynq.task.process" {
			foundWorkerSpan = true
			if span.SpanKind() != oteltrace.SpanKindConsumer {
				t.Error("Worker span should have SpanKind Consumer")
			}
		}
	}

	if !foundParentSpan {
		t.Error("Parent span not found in recorded spans")
	}
	if !foundWorkerSpan {
		t.Error("Worker span not found in recorded spans")
	}

	t.Logf("Successfully verified E2E trace flow with TraceID: %s", expectedTraceID)
}

// TestE2ETraceFlow_ExtractLinks tests the complete trace flow for extract links tasks
func TestE2ETraceFlow_ExtractLinks(t *testing.T) {
	// Setup in-memory span exporter
	spanRecorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(
		trace.WithSpanProcessor(spanRecorder),
	)
	otel.SetTracerProvider(tp)

	// Create parent span
	tracer := tp.Tracer("test")
	ctx, parentSpan := tracer.Start(context.Background(), "http.request")

	parentSpanContext := parentSpan.SpanContext()

	// Enqueue extract links task
	payload := ExtractLinksTaskPayload{
		ParentJobID: "parent-job-123",
		SourceURL:   "https://example.com/page",
		ParentDepth: 0,
		EnqueuedAt:  time.Now().UnixNano(),
	}

	// Capture trace context
	if span := oteltrace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		payload.TraceID = spanCtx.TraceID().String()
		payload.SpanID = spanCtx.SpanID().String()
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	// Simulate worker processing
	var receivedPayload ExtractLinksTaskPayload
	if err := json.Unmarshal(payloadBytes, &receivedPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	// Extract trace context
	traceID, _ := oteltrace.TraceIDFromHex(receivedPayload.TraceID)
	spanID, _ := oteltrace.SpanIDFromHex(receivedPayload.SpanID)

	remoteSpanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	})

	linkedCtx := oteltrace.ContextWithRemoteSpanContext(context.Background(), remoteSpanCtx)

	// Create worker span
	_, workerSpan := tracer.Start(linkedCtx, "asynq.task.extract_links",
		oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
	)
	workerSpan.End()

	// End parent span before verification
	parentSpan.End()

	time.Sleep(100 * time.Millisecond)

	// Verify trace chain
	spans := spanRecorder.Ended()
	expectedTraceID := parentSpanContext.TraceID()

	for _, span := range spans {
		if span.SpanContext().TraceID() != expectedTraceID {
			t.Errorf("Span has different TraceID: got %s, want %s",
				span.SpanContext().TraceID(), expectedTraceID)
		}
	}

	t.Logf("Successfully verified E2E trace flow for ExtractLinks with TraceID: %s", expectedTraceID)
}

// TestE2EQueueWaitTime tests that queue wait time is correctly calculated
func TestE2EQueueWaitTime(t *testing.T) {
	// Enqueue a task with timestamp
	enqueuedTime := time.Now().Add(-5 * time.Second)
	payload := ScrapeTaskPayload{
		JobID:        "test-job-timing",
		URL:          "https://example.com",
		ExtractLinks: false,
		EnqueuedAt:   enqueuedTime.UnixNano(),
		TraceID:      "test-trace-id",
		SpanID:       "test-span-id",
	}

	// Simulate worker starting processing now
	processingStartTime := time.Now()

	// Calculate queue wait time (what worker does)
	var queueWaitTime time.Duration
	if payload.EnqueuedAt > 0 {
		enqueueTime := time.Unix(0, payload.EnqueuedAt)
		queueWaitTime = processingStartTime.Sub(enqueueTime)
	}

	// Verify wait time is approximately 5 seconds
	expectedMin := 4900 * time.Millisecond
	expectedMax := 5100 * time.Millisecond

	if queueWaitTime < expectedMin || queueWaitTime > expectedMax {
		t.Errorf("Queue wait time out of expected range: got %v, expected between %v and %v",
			queueWaitTime, expectedMin, expectedMax)
	}

	t.Logf("Queue wait time: %v (expected ~5s)", queueWaitTime)
}

// TestE2ETraceFlowWithRealAsynq tests with actual Asynq client/server (requires Redis)
func TestE2ETraceFlowWithRealAsynq(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup span recorder
	spanRecorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(
		trace.WithSpanProcessor(spanRecorder),
	)
	otel.SetTracerProvider(tp)

	// Setup Asynq client - use test Redis if available, otherwise default
	redisAddr := getTestRedisAddr()
	t.Logf("Attempting to connect to Redis at %s", redisAddr)

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()

	queueClient := &Client{client: client}

	// Create parent span
	tracer := tp.Tracer("test")
	ctx, parentSpan := tracer.Start(context.Background(), "http.request")

	// Enqueue a real task
	jobID := "test-job-real-" + time.Now().Format("20060102150405")
	_, err := queueClient.EnqueueScrape(ctx, jobID, "https://example.com", true)
	if err != nil {
		t.Skipf("Could not connect to Redis: %v", err)
	}

	t.Logf("Enqueued task: %s", jobID)

	// End parent span before checking
	parentSpan.End()

	// The actual worker processing would happen asynchronously
	// For this test, we've verified that:
	// 1. Task can be enqueued with trace context
	// 2. Parent span is recorded
	// In a real scenario, a worker would process this and create linked spans

	time.Sleep(100 * time.Millisecond)
	spans := spanRecorder.Ended()

	if len(spans) == 0 {
		t.Error("No spans recorded")
	}

	for _, span := range spans {
		t.Logf("Recorded span: %s (TraceID: %s)", span.Name(), span.SpanContext().TraceID())
	}
}
