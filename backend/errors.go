package main

type ErrorCode string

// Enhanced canonical error codes for FinTech operations
const (
	// Client-side errors (non-retryable)
	ErrInvalidRequest     ErrorCode = "INVALID_REQUEST"
	ErrPaymentIDRequired  ErrorCode = "PAYMENT_ID_REQUIRED"
	ErrPaymentKeyNotFound ErrorCode = "PAYMENT_KEY_NOT_FOUND"
	ErrPaymentIDMismatch  ErrorCode = "PAYMENT_ID_MISMATCH"
	ErrInsufficientFunds  ErrorCode = "INSUFFICIENT_FUNDS"
	ErrCardDeclined       ErrorCode = "CARD_DECLINED"
	ErrAuthFailed         ErrorCode = "AUTHENTICATION_FAILED"

	// Provider errors (retryable)
	ErrNoHealthyServers   ErrorCode = "NO_HEALTHY_SERVERS"
	ErrGatewayUnavailable ErrorCode = "GATEWAY_UNAVAILABLE"
	ErrGatewayTimeout     ErrorCode = "GATEWAY_TIMEOUT"
	ErrProviderError      ErrorCode = "PROVIDER_ERROR"
	ErrRateLimited        ErrorCode = "RATE_LIMITED"
	ErrProviderDown       ErrorCode = "PROVIDER_DOWN"

	// Network errors
	ErrConnectionReset   ErrorCode = "CONNECTION_RESET"
	ErrConnectionTimeout ErrorCode = "CONNECTION_TIMEOUT"
	ErrNetworkError      ErrorCode = "NETWORK_ERROR"
	ErrDNSError          ErrorCode = "DNS_ERROR"

	// Response errors
	ErrMalformedResponse ErrorCode = "MALFORMED_RESPONSE"
	ErrEmptyResponse     ErrorCode = "EMPTY_RESPONSE"
	ErrSlowResponse      ErrorCode = "SLOW_RESPONSE"
	ErrInvalidJSON       ErrorCode = "INVALID_JSON"

	// System errors
	ErrInternalError ErrorCode = "INTERNAL_ERROR"
	ErrDatabaseError ErrorCode = "DATABASE_ERROR"
	ErrCircuitOpen   ErrorCode = "CIRCUIT_OPEN"
	ErrPanic         ErrorCode = "PANIC"

	// Compliance errors
	ErrComplianceFailed ErrorCode = "COMPLIANCE_FAILED"
	ErrKYCRequired      ErrorCode = "KYC_REQUIRED"
)

type ErrorResponse struct {
	Success   bool      `json:"success"`
	ErrorCode ErrorCode `json:"error_code"`
	Message   string    `json:"message"`
	Status    string    `json:"status,omitempty"`
	Details   string    `json:"details,omitempty"`
}

type SuccessResponse struct {
	Success   bool        `json:"success"`
	Status    string      `json:"status"`
	PaymentID string      `json:"payment_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

func NewErrorResponse(code ErrorCode, message string, status string, details string) ErrorResponse {
	return ErrorResponse{
		Success:   false,
		ErrorCode: code,
		Message:   message,
		Status:    status,
		Details:   details,
	}
}

func NewSuccessResponse(status string, paymentID string, data interface{}) SuccessResponse {
	return SuccessResponse{
		Success:   true,
		Status:    status,
		PaymentID: paymentID,
		Data:      data,
	}
}
