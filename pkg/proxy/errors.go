package proxy

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/vango-go/vai/pkg/core"
	"github.com/vango-go/vai/pkg/core/types"
)

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(ContextKeyRequestID).(string); ok {
		return id
	}
	return ""
}

func statusForError(err *core.Error) int {
	switch err.Type {
	case core.ErrInvalidRequest:
		return http.StatusBadRequest
	case core.ErrAuthentication:
		return http.StatusUnauthorized
	case core.ErrPermission:
		return http.StatusForbidden
	case core.ErrNotFound:
		return http.StatusNotFound
	case core.ErrRateLimit:
		return http.StatusTooManyRequests
	case core.ErrOverloaded:
		return http.StatusServiceUnavailable
	case core.ErrProvider:
		return http.StatusBadGateway
	case core.ErrAPI:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func writeJSONError(w http.ResponseWriter, err *core.Error, requestID string) {
	if err == nil {
		return
	}
	if err.RequestID == "" && requestID != "" {
		err.RequestID = requestID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusForError(err))
	json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": err,
	})
}

func writeJSONErrorWithStatus(w http.ResponseWriter, status int, err *core.Error, requestID string) {
	if err == nil {
		return
	}
	if err.RequestID == "" && requestID != "" {
		err.RequestID = requestID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": err,
	})
}

func normalizeCoreError(err error) *core.Error {
	if err == nil {
		return nil
	}
	if apiErr, ok := err.(*core.Error); ok {
		return apiErr
	}
	return core.NewAPIError(err.Error())
}

func streamErrorEvent(err *core.Error) types.ErrorEvent {
	event := types.ErrorEvent{
		Type: "error",
	}
	if err == nil {
		event.Error = types.Error{
			Type:    string(core.ErrAPI),
			Message: "unknown error",
		}
		return event
	}
	event.Error = types.Error{
		Type:    string(err.Type),
		Message: err.Message,
	}
	return event
}
