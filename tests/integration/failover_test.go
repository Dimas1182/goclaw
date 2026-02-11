package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/types"
)

// TestProviderFailover tests provider failover functionality
func TestProviderFailover(t *testing.T) {
	errorClassifier := types.NewSimpleErrorClassifier()

	// Create mock providers that simulate failures
	primary := &mockProvider{
		shouldFail: true,
		failReason: types.FailoverReasonAuth,
	}

	fallback := &mockProvider{
		shouldFail: false,
	}

	// Create failover provider
	failover := providers.NewFailoverProvider(primary, fallback, errorClassifier)

	ctx := context.Background()
	messages := []providers.Message{
		{Role: "user", Content: "Test message"},
	}

	// First call should failover to fallback
	response, err := failover.Chat(ctx, messages, nil)
	if err != nil {
		t.Fatalf("Expected successful failover, got error: %v", err)
	}

	if response.Content != "fallback response" {
		t.Errorf("Expected fallback response, got: %s", response.Content)
	}

	// Check circuit breaker state
	cb := failover.GetCircuitBreaker()
	state := cb.GetState()
	if state != providers.CircuitStateOpen {
		t.Logf("Circuit breaker state: %v (expected Open after failures)", state)
	}

	t.Log("Provider failover test passed")
}

// TestProviderRotation tests provider profile rotation
func TestProviderRotation(t *testing.T) {
	errorClassifier := types.NewSimpleErrorClassifier()

	// Create rotation provider with round-robin strategy
	rotation := providers.NewRotationProvider(
		providers.RotationStrategyRoundRobin,
		5*time.Minute,
		errorClassifier,
	)

	// Add multiple profiles
	for i := 0; i < 3; i++ {
		prof := &mockProvider{
			shouldFail: false,
			response:   fmt.Sprintf("profile-%d-response", i),
		}
		rotation.AddProfile(fmt.Sprintf("profile-%d", i), prof, "key", 1)
	}

	ctx := context.Background()
	messages := []providers.Message{
		{Role: "user", Content: "Test message"},
	}

	// Test rotation - should cycle through profiles
	responses := []string{}
	for i := 0; i < 3; i++ {
		response, err := rotation.Chat(ctx, messages, nil)
		if err != nil {
			t.Fatalf("Iteration %d failed: %v", i, err)
		}
		responses = append(responses, response.Content)
	}

	// Verify we got different responses (round-robin working)
	if responses[0] == responses[1] && responses[1] == responses[2] {
		t.Error("Expected different responses from rotation, got all same")
	}

	t.Logf("Rotation responses: %v", responses)
	t.Log("Provider rotation test passed")
}

// TestProviderCooldown tests cooldown mechanism for failed providers
func TestProviderCooldown(t *testing.T) {
	errorClassifier := types.NewSimpleErrorClassifier()

	rotation := providers.NewRotationProvider(
		providers.RotationStrategyLeastUsed,
		1*time.Minute, // Short cooldown for testing
		errorClassifier,
	)

	// Add profiles
	prof1 := &mockProvider{shouldFail: true, failReason: types.FailoverReasonRateLimit}
	prof2 := &mockProvider{shouldFail: false, response: "profile-2-response"}

	rotation.AddProfile("profile-1", prof1, "key1", 1)
	rotation.AddProfile("profile-2", prof2, "key2", 1)

	ctx := context.Background()
	messages := []providers.Message{
		{Role: "user", Content: "Test message"},
	}

	// First call should use profile-1 and fail
	_, err := rotation.Chat(ctx, messages, nil)
	if err == nil {
		t.Error("Expected first call to fail")
	}

	// Check profile-1 status
	status, _ := rotation.GetProfileStatus("profile-1")
	if !status["in_cooldown"].(bool) {
		t.Error("Expected profile-1 to be in cooldown")
	}

	// Second call should skip profile-1 and use profile-2
	response, err := rotation.Chat(ctx, messages, nil)
	if err != nil {
		t.Fatalf("Expected second call to succeed with profile-2: %v", err)
	}

	if response.Content != "profile-2-response" {
		t.Errorf("Expected profile-2 response, got: %s", response.Content)
	}

	t.Log("Provider cooldown test passed")
}

// TestCircuitBreaker tests circuit breaker behavior
func TestCircuitBreaker(t *testing.T) {
	// Create circuit breaker with threshold of 3 failures
	cb := providers.NewCircuitBreaker(3, 5*time.Second)

	// Initially should be closed
	if cb.GetState() != providers.CircuitStateClosed {
		t.Errorf("Expected initial state to be Closed, got: %v", cb.GetState())
	}

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Should now be open
	if cb.GetState() != providers.CircuitStateOpen {
		t.Errorf("Expected state to be Open after threshold failures, got: %v", cb.GetState())
	}

	// Requests should be denied
	if cb.AllowRequest() {
		t.Error("Expected requests to be denied when circuit is open")
	}

	// Wait for timeout and verify state change
	time.Sleep(6 * time.Second)

	// After timeout, should allow request (transition to half-open)
	if !cb.AllowRequest() {
		t.Error("Expected requests to be allowed after timeout")
	}

	// Record success to close circuit
	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.GetState() != providers.CircuitStateClosed {
		t.Errorf("Expected state to be Closed after successes, got: %v", cb.GetState())
	}

	t.Log("Circuit breaker test passed")
}

// TestErrorClassification tests error classification
func TestErrorClassification(t *testing.T) {
	classifier := types.NewSimpleErrorClassifier()

	tests := []struct {
		errorString string
		expected    types.FailoverReason
	}{
		{"invalid api key", types.FailoverReasonAuth},
		{"401 unauthorized", types.FailoverReasonAuth},
		{"rate limit exceeded", types.FailoverReasonRateLimit},
		{"429 too many requests", types.FailoverReasonRateLimit},
		{"payment required", types.FailoverReasonBilling},
		{"context deadline exceeded", types.FailoverReasonTimeout},
	}

	for _, tt := range tests {
		reason := classifier.ClassifyError(errors.New(tt.errorString))
		if reason != tt.expected {
			t.Errorf("Error '%s' classified as %v, expected %v",
				tt.errorString, reason, tt.expected)
		}
	}

	t.Log("Error classification test passed")
}

// Mock provider for testing
type mockProvider struct {
	shouldFail bool
	failReason types.FailoverReason
	response   string
	callCount  int
}

func (m *mockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, options ...providers.ChatOption) (*providers.Response, error) {
	m.callCount++

	if m.shouldFail {
		var errMsg string
		switch m.failReason {
		case types.FailoverReasonAuth:
			errMsg = "401 unauthorized: invalid api key"
		case types.FailoverReasonRateLimit:
			errMsg = "429 rate limit exceeded"
		case types.FailoverReasonBilling:
			errMsg = "402 payment required"
		default:
			errMsg = "unknown error"
		}
		return nil, errors.New(errMsg)
	}

	if m.response == "" {
		m.response = "mock response"
	}

	return &providers.Response{
		Content: m.response,
		Usage: providers.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}, nil
}

func (m *mockProvider) ChatWithTools(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, options ...providers.ChatOption) (*providers.Response, error) {
	return m.Chat(ctx, messages, tools, options...)
}

func (m *mockProvider) Close() error {
	return nil
}
