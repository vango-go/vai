package vai

import (
	"github.com/vango-go/vai/pkg/core"
)

// SDK-level error type that wraps core errors
type Error = core.Error

// Error types
const (
	ErrInvalidRequest = core.ErrInvalidRequest
	ErrAuthentication = core.ErrAuthentication
	ErrPermission     = core.ErrPermission
	ErrNotFound       = core.ErrNotFound
	ErrRateLimit      = core.ErrRateLimit
	ErrAPI            = core.ErrAPI
	ErrOverloaded     = core.ErrOverloaded
	ErrProvider       = core.ErrProvider
)

// Error constructors
var (
	NewInvalidRequestError = core.NewInvalidRequestError
	NewAuthenticationError = core.NewAuthenticationError
	NewRateLimitError      = core.NewRateLimitError
	NewProviderError       = core.NewProviderError
)
