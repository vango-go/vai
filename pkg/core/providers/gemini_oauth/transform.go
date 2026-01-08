package gemini_oauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

var debugTransform = os.Getenv("DEBUG_GEMINI_STREAM") != ""

// WrapRequest wraps a Gemini API request body in the Cloud Code Assist format.
// Input: standard Gemini request body (JSON bytes)
// Output: wrapped body {project, model, request: {...}}
func WrapRequest(body []byte, projectID, model string) ([]byte, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty request body")
	}

	var originalBody map[string]any
	if err := json.Unmarshal(body, &originalBody); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	// Handle system_instruction -> systemInstruction rename (camelCase for Cloud Code API)
	if sysInst, ok := originalBody["system_instruction"]; ok {
		originalBody["systemInstruction"] = sysInst
		delete(originalBody, "system_instruction")
	}

	// Remove model from original body if present (we put it at top level of wrapper)
	delete(originalBody, "model")

	// Apply model fallbacks
	effectiveModel := model
	if fallback, ok := ModelFallbacks[model]; ok {
		effectiveModel = fallback
	}

	// Wrap in Cloud Code Assist format
	wrappedBody := map[string]any{
		"project": projectID,
		"model":   effectiveModel,
		"request": originalBody,
	}

	return json.Marshal(wrappedBody)
}

// UnwrapResponse extracts the inner response from a Cloud Code Assist wrapper.
// Input: {response: {...}} or direct response
// Output: inner {...} or original if not wrapped
func UnwrapResponse(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}

	var wrapped map[string]any
	if err := json.Unmarshal(body, &wrapped); err != nil {
		// If we can't parse it, return as-is
		return body, nil
	}

	// Check if wrapped with "response" field
	if inner, ok := wrapped["response"]; ok {
		return json.Marshal(inner)
	}

	// Not wrapped, return as-is
	return body, nil
}

// TransformingReader wraps an io.ReadCloser and unwraps Cloud Code SSE responses.
// Transforms: data: {"response": {...}} -> data: {...}
type TransformingReader struct {
	source     io.ReadCloser
	buffer     bytes.Buffer
	lineBuffer strings.Builder
	done       bool
}

// NewTransformingReader creates a reader that unwraps Cloud Code SSE format.
func NewTransformingReader(source io.ReadCloser) *TransformingReader {
	return &TransformingReader{
		source: source,
	}
}

// Read implements io.Reader, transforming SSE data on the fly.
func (r *TransformingReader) Read(p []byte) (int, error) {
	// First, drain any buffered output
	if r.buffer.Len() > 0 {
		return r.buffer.Read(p)
	}

	if r.done {
		return 0, io.EOF
	}

	// Read from source
	buf := make([]byte, 4096)
	n, err := r.source.Read(buf)

	if n > 0 {
		r.lineBuffer.Write(buf[:n])

		// Process complete lines
		content := r.lineBuffer.String()
		lines := strings.Split(content, "\n")

		// Keep incomplete last line in buffer
		r.lineBuffer.Reset()
		if !strings.HasSuffix(content, "\n") && len(lines) > 0 {
			r.lineBuffer.WriteString(lines[len(lines)-1])
			lines = lines[:len(lines)-1]
		}

		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)

			if trimmedLine == "" {
				r.buffer.WriteString("\n")
				continue
			}

			if strings.HasPrefix(trimmedLine, "data:") {
				jsonData := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "data:"))
				if debugTransform {
					log.Printf("[TRANSFORM] Raw SSE data: %s", jsonData)
				}
				if jsonData == "" {
					r.buffer.WriteString("data:\n")
					continue
				}

				// Try to unwrap
				var wrapped map[string]any
				if err := json.Unmarshal([]byte(jsonData), &wrapped); err == nil {
					if inner, ok := wrapped["response"]; ok {
						innerBytes, _ := json.Marshal(inner)
						if debugTransform {
							log.Printf("[TRANSFORM] Unwrapped: %s", string(innerBytes))
						}
						r.buffer.WriteString("data: ")
						r.buffer.Write(innerBytes)
						r.buffer.WriteString("\n")
						continue
					} else if debugTransform {
						log.Printf("[TRANSFORM] No 'response' field in: %v", wrapped)
					}
				} else if debugTransform {
					log.Printf("[TRANSFORM] Failed to parse: %v", err)
				}
			}

			// Pass through unchanged
			r.buffer.WriteString(line)
			r.buffer.WriteString("\n")
		}
	}

	if err == io.EOF {
		r.done = true
		// Process any remaining content in lineBuffer
		if r.lineBuffer.Len() > 0 {
			remaining := r.lineBuffer.String()
			if strings.HasPrefix(strings.TrimSpace(remaining), "data:") {
				jsonData := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(remaining), "data:"))
				var wrapped map[string]any
				if json.Unmarshal([]byte(jsonData), &wrapped) == nil {
					if inner, ok := wrapped["response"]; ok {
						innerBytes, _ := json.Marshal(inner)
						r.buffer.WriteString("data: ")
						r.buffer.Write(innerBytes)
						r.buffer.WriteString("\n")
					} else {
						r.buffer.WriteString(remaining)
					}
				} else {
					r.buffer.WriteString(remaining)
				}
			} else {
				r.buffer.WriteString(remaining)
			}
			r.lineBuffer.Reset()
		}
	}

	if err != nil && err != io.EOF {
		return 0, err
	}

	// Return buffered content
	if r.buffer.Len() > 0 {
		return r.buffer.Read(p)
	}

	if r.done {
		return 0, io.EOF
	}

	return 0, nil
}

// Close closes the underlying reader.
func (r *TransformingReader) Close() error {
	return r.source.Close()
}
