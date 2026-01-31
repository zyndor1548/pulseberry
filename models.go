package main

import (
	"time"
)

// Canonical domain models for provider-agnostic operations

// PaymentRequest represents a normalized payment request
type PaymentRequest struct {
	ID             string                 `json:"id" validate:"required"`
	Amount         int64                  `json:"amount" validate:"required,gt=0"`
	Currency       string                 `json:"currency" validate:"required,len=3"`
	Description    string                 `json:"description,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key" validate:"required"`
	UserID         string                 `json:"user_id,omitempty"`
	Email          string                 `json:"email,omitempty"`
}

// PaymentResponse represents a normalized payment response
type PaymentResponse struct {
	PaymentID     string                 `json:"payment_id"`
	Status        PaymentStatus          `json:"status"`
	ProviderTxnID string                 `json:"provider_txn_id,omitempty"`
	Provider      string                 `json:"provider"`
	LatencyMs     int64                  `json:"latency_ms"`
	ProcessedAt   time.Time              `json:"processed_at"`
	ErrorCode     *CanonicalErrorCode    `json:"error_code,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// PaymentStatus represents the status of a payment
type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "PENDING"
	PaymentStatusProcessing PaymentStatus = "PROCESSING"
	PaymentStatusSuccess    PaymentStatus = "SUCCESS"
	PaymentStatusFailed     PaymentStatus = "FAILED"
	PaymentStatusCancelled  PaymentStatus = "CANCELLED"
)

// RefundRequest represents a normalized refund request
type RefundRequest struct {
	ID             string `json:"id" validate:"required"`
	PaymentID      string `json:"payment_id" validate:"required"`
	Amount         int64  `json:"amount" validate:"required,gt=0"`
	Reason         string `json:"reason,omitempty"`
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
}

// RefundResponse represents a normalized refund response
type RefundResponse struct {
	RefundID     string                 `json:"refund_id"`
	Status       string                 `json:"status"`
	Provider     string                 `json:"provider"`
	ProcessedAt  time.Time              `json:"processed_at"`
	ErrorCode    *CanonicalErrorCode    `json:"error_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ComplianceCheckRequest represents a KYC/AML compliance check
type ComplianceCheckRequest struct {
	UserID         string                 `json:"user_id" validate:"required"`
	CheckType      ComplianceCheckType    `json:"check_type" validate:"required"`
	DocumentData   map[string]interface{} `json:"document_data,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key" validate:"required"`
}

// ComplianceCheckType defines types of compliance checks
type ComplianceCheckType string

const (
	ComplianceCheckKYC ComplianceCheckType = "KYC"
	ComplianceCheckAML ComplianceCheckType = "AML"
)

// ComplianceCheckResponse represents compliance check results
type ComplianceCheckResponse struct {
	CheckID      string                 `json:"check_id"`
	Status       ComplianceStatus       `json:"status"`
	RiskLevel    string                 `json:"risk_level,omitempty"`
	Provider     string                 `json:"provider"`
	ProcessedAt  time.Time              `json:"processed_at"`
	ErrorCode    *CanonicalErrorCode    `json:"error_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ComplianceStatus represents compliance check status
type ComplianceStatus string

const (
	ComplianceStatusApproved ComplianceStatus = "APPROVED"
	ComplianceStatusRejected ComplianceStatus = "REJECTED"
	ComplianceStatusPending  ComplianceStatus = "PENDING"
	ComplianceStatusReview   ComplianceStatus = "REVIEW_REQUIRED"
)

// BNPLRequest represents a Buy Now Pay Later request
type BNPLRequest struct {
	ID             string                 `json:"id" validate:"required"`
	Amount         int64                  `json:"amount" validate:"required,gt=0"`
	Currency       string                 `json:"currency" validate:"required,len=3"`
	CustomerEmail  string                 `json:"customer_email" validate:"required,email"`
	Term           int                    `json:"term"` // Number of installments
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key" validate:"required"`
}

// BNPLResponse represents a BNPL response
type BNPLResponse struct {
	BNPLID       string                 `json:"bnpl_id"`
	Status       string                 `json:"status"`
	Provider     string                 `json:"provider"`
	ApprovalURL  string                 `json:"approval_url,omitempty"`
	ProcessedAt  time.Time              `json:"processed_at"`
	ErrorCode    *CanonicalErrorCode    `json:"error_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// CanonicalErrorCode represents normalized error taxonomy
type CanonicalErrorCode string

const (
	// Client errors
	ErrCodeInvalidRequest     CanonicalErrorCode = "INVALID_REQUEST"
	ErrCodeInsufficientFunds  CanonicalErrorCode = "INSUFFICIENT_FUNDS"
	ErrCodeCardDeclined       CanonicalErrorCode = "CARD_DECLINED"
	ErrCodeAuthenticationFail CanonicalErrorCode = "AUTHENTICATION_FAILED"

	// Provider errors (retryable)
	ErrCodeRateLimited      CanonicalErrorCode = "RATE_LIMITED"
	ErrCodeProviderError    CanonicalErrorCode = "PROVIDER_ERROR"
	ErrCodeProviderTimeout  CanonicalErrorCode = "PROVIDER_TIMEOUT"
	ErrCodeProviderDown     CanonicalErrorCode = "PROVIDER_DOWN"
	ErrCodeProviderDegraded CanonicalErrorCode = "PROVIDER_DEGRADED"

	// Network errors (retryable)
	ErrCodeNetworkError CanonicalErrorCode = "NETWORK_ERROR"
	ErrCodeTimeout      CanonicalErrorCode = "TIMEOUT"

	// System errors
	ErrCodeInternalError CanonicalErrorCode = "INTERNAL_ERROR"
	ErrCodeCircuitOpen   CanonicalErrorCode = "CIRCUIT_OPEN"

	// Compliance errors
	ErrCodeComplianceFailed CanonicalErrorCode = "COMPLIANCE_FAILED"
	ErrCodeKYCRequired      CanonicalErrorCode = "KYC_REQUIRED"
)

// ErrorClassification defines error retry behavior
type ErrorClassification string

const (
	ErrorClassRetryable  ErrorClassification = "RETRYABLE"
	ErrorClassFatal      ErrorClassification = "FATAL"
	ErrorClassDegraded   ErrorClassification = "DEGRADED"
	ErrorClassClientSide ErrorClassification = "CLIENT_SIDE"
)

// ClassifyError determines if an error is retryable
func ClassifyError(code CanonicalErrorCode) ErrorClassification {
	switch code {
	case ErrCodeRateLimited, ErrCodeProviderError, ErrCodeProviderTimeout,
		ErrCodeProviderDown, ErrCodeNetworkError, ErrCodeTimeout:
		return ErrorClassRetryable
	case ErrCodeInvalidRequest, ErrCodeInsufficientFunds, ErrCodeCardDeclined,
		ErrCodeAuthenticationFail, ErrCodeComplianceFailed, ErrCodeKYCRequired:
		return ErrorClassClientSide
	case ErrCodeProviderDegraded:
		return ErrorClassDegraded
	default:
		return ErrorClassFatal
	}
}

// ProviderCapabilities defines what features a provider supports
type ProviderCapabilities struct {
	SupportsRefunds     bool     `json:"supports_refunds"`
	SupportsBNPL        bool     `json:"supports_bnpl"`
	ComplianceReady     bool     `json:"compliance_ready"`
	MaxAmountCents      int64    `json:"max_amount_cents"`
	MinAmountCents      int64    `json:"min_amount_cents"`
	SupportedCurrencies []string `json:"supported_currencies"`
	SupportedRegions    []string `json:"supported_regions"`
}

// HealthStatus represents provider health check results
type HealthStatus struct {
	Healthy   bool      `json:"healthy"`
	Timestamp time.Time `json:"timestamp"`
	Latency   int64     `json:"latency_ms"`
	Message   string    `json:"message,omitempty"`
}
