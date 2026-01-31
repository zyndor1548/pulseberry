package main

import (
	"context"
	"errors"
	"fmt"
)

// Provider defines the interface all payment provider adapters must implement
type Provider interface {
	Name() string
	Charge(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error)
	Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error)
	HealthCheck(ctx context.Context) (*HealthStatus, error)
	Capabilities() ProviderCapabilities
}

// ProviderError wraps provider-specific errors with normalized codes
type ProviderError struct {
	CanonicalCode CanonicalErrorCode
	ProviderCode  string
	Message       string
	Retryable     bool
	OriginalError error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("[%s] %s (provider_code: %s)", e.CanonicalCode, e.Message, e.ProviderCode)
}

func NewProviderError(canonical CanonicalErrorCode, providerCode, message string, original error) *ProviderError {
	classification := ClassifyError(canonical)
	return &ProviderError{
		CanonicalCode: canonical,
		ProviderCode:  providerCode,
		Message:       message,
		Retryable:     classification == ErrorClassRetryable,
		OriginalError: original,
	}
}

// BaseProvider provides common functionality for all providers
type BaseProvider struct {
	name         string
	capabilities ProviderCapabilities
}

func (bp *BaseProvider) Name() string {
	return bp.name
}

func (bp *BaseProvider) Capabilities() ProviderCapabilities {
	return bp.capabilities
}

// MockStripeProvider simulates Stripe payment provider
type MockStripeProvider struct {
	BaseProvider
	baseURL string
}

func NewMockStripeProvider(baseURL string) *MockStripeProvider {
	return &MockStripeProvider{
		BaseProvider: BaseProvider{
			name: "stripe",
			capabilities: ProviderCapabilities{
				SupportsRefunds:     true,
				SupportsBNPL:        false,
				ComplianceReady:     true,
				MaxAmountCents:      99999999, // $999,999.99
				MinAmountCents:      50,       // $0.50
				SupportedCurrencies: []string{"USD", "EUR", "GBP", "INR"},
				SupportedRegions:    []string{"US", "EU", "IN"},
			},
		},
		baseURL: baseURL,
	}
}

func (p *MockStripeProvider) Charge(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	// Validate amount against capabilities
	if req.Amount < p.capabilities.MinAmountCents {
		return nil, NewProviderError(
			ErrCodeInvalidRequest,
			"amount_too_small",
			fmt.Sprintf("Amount must be at least %d cents", p.capabilities.MinAmountCents),
			nil,
		)
	}

	if req.Amount > p.capabilities.MaxAmountCents {
		return nil, NewProviderError(
			ErrCodeInvalidRequest,
			"amount_too_large",
			fmt.Sprintf("Amount must not exceed %d cents", p.capabilities.MaxAmountCents),
			nil,
		)
	}

	// In real implementation, make HTTP request to Stripe API
	// For now, this is a mock pass-through to existing gateway
	return nil, errors.New("not yet implemented - use legacy gateway")
}

func (p *MockStripeProvider) Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	if !p.capabilities.SupportsRefunds {
		return nil, NewProviderError(
			ErrCodeInvalidRequest,
			"refunds_not_supported",
			"This provider does not support refunds",
			nil,
		)
	}

	// Mock implementation
	return nil, errors.New("not yet implemented")
}

func (p *MockStripeProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	// In real implementation, ping Stripe health endpoint
	return &HealthStatus{
		Healthy: true,
	}, nil
}

// MockRazorpayProvider simulates Razorpay payment provider
type MockRazorpayProvider struct {
	BaseProvider
	baseURL string
}

func NewMockRazorpayProvider(baseURL string) *MockRazorpayProvider {
	return &MockRazorpayProvider{
		BaseProvider: BaseProvider{
			name: "razorpay",
			capabilities: ProviderCapabilities{
				SupportsRefunds:     true,
				SupportsBNPL:        false,
				ComplianceReady:     true,
				MaxAmountCents:      10000000, // 1,00,000 INR
				MinAmountCents:      100,      // 1 INR
				SupportedCurrencies: []string{"INR"},
				SupportedRegions:    []string{"IN"},
			},
		},
		baseURL: baseURL,
	}
}

func (p *MockRazorpayProvider) Charge(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	// Validate currency
	supported := false
	for _, curr := range p.capabilities.SupportedCurrencies {
		if curr == req.Currency {
			supported = true
			break
		}
	}

	if !supported {
		return nil, NewProviderError(
			ErrCodeInvalidRequest,
			"currency_not_supported",
			fmt.Sprintf("Currency %s is not supported by this provider", req.Currency),
			nil,
		)
	}

	return nil, errors.New("not yet implemented - use legacy gateway")
}

func (p *MockRazorpayProvider) Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	return nil, errors.New("not yet implemented")
}

func (p *MockRazorpayProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{
		Healthy: true,
	}, nil
}

// MockKlarnaProvider simulates Klarna BNPL provider
type MockKlarnaProvider struct {
	BaseProvider
	baseURL string
}

func NewMockKlarnaProvider(baseURL string) *MockKlarnaProvider {
	return &MockKlarnaProvider{
		BaseProvider: BaseProvider{
			name: "klarna",
			capabilities: ProviderCapabilities{
				SupportsRefunds:     true,
				SupportsBNPL:        true,
				ComplianceReady:     false,
				MaxAmountCents:      1000000, // $10,000
				MinAmountCents:      1000,    // $10
				SupportedCurrencies: []string{"USD", "EUR", "GBP", "SEK"},
				SupportedRegions:    []string{"US", "EU"},
			},
		},
		baseURL: baseURL,
	}
}

func (p *MockKlarnaProvider) Charge(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	return nil, errors.New("not yet implemented")
}

func (p *MockKlarnaProvider) Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	return nil, errors.New("not yet implemented")
}

func (p *MockKlarnaProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{
		Healthy: true,
	}, nil
}

// ComplianceProvider defines interface for KYC/AML providers
type ComplianceProvider interface {
	Name() string
	CheckKYC(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceCheckResponse, error)
	CheckAML(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceCheckResponse, error)
	HealthCheck(ctx context.Context) (*HealthStatus, error)
}

// MockOnfidoProvider simulates Onfido compliance provider
type MockOnfidoProvider struct {
	name    string
	baseURL string
}

func NewMockOnfidoProvider(baseURL string) *MockOnfidoProvider {
	return &MockOnfidoProvider{
		name:    "onfido",
		baseURL: baseURL,
	}
}

func (p *MockOnfidoProvider) Name() string {
	return p.name
}

func (p *MockOnfidoProvider) CheckKYC(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceCheckResponse, error) {
	// Mock implementation - in production, call Onfido API
	return &ComplianceCheckResponse{
		CheckID:  fmt.Sprintf("kyc_%s", req.UserID),
		Status:   ComplianceStatusApproved,
		Provider: p.name,
	}, nil
}

func (p *MockOnfidoProvider) CheckAML(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceCheckResponse, error) {
	// Mock implementation
	return &ComplianceCheckResponse{
		CheckID:  fmt.Sprintf("aml_%s", req.UserID),
		Status:   ComplianceStatusApproved,
		Provider: p.name,
	}, nil
}

func (p *MockOnfidoProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{
		Healthy: true,
	}, nil
}
