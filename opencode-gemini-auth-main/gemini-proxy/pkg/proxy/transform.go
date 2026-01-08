package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var modelActionRegex = regexp.MustCompile(`/v1(?:beta)?/models/([^:]+):(\w+)`)

// TransformRequest converts a standard Gemini API request to Cloud Code Assist format.
func TransformRequest(r *http.Request, projectID, accessToken string) (*http.Request, bool, error) {
	// Parse the model and action from the URL path
	matches := modelActionRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 3 {
		return nil, false, fmt.Errorf("invalid request path: %s", r.URL.Path)
	}

	rawModel := matches[1]
	action := matches[2]

	// Apply model fallbacks
	effectiveModel := rawModel
	if fallback, ok := ModelFallbacks[rawModel]; ok {
		effectiveModel = fallback
	}

	streaming := action == "streamGenerateContent"

	// Build the target URL
	targetURL := fmt.Sprintf("%s/v1internal:%s", CloudCodeAssistEndpoint, action)
	if streaming {
		targetURL += "?alt=sse"
	}

	// Read and transform the body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read request body: %w", err)
	}
	r.Body.Close()

	var originalBody map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &originalBody); err != nil {
			return nil, false, fmt.Errorf("failed to parse request body: %w", err)
		}
	}

	// Handle system_instruction -> systemInstruction rename
	if sysInst, ok := originalBody["system_instruction"]; ok {
		originalBody["systemInstruction"] = sysInst
		delete(originalBody, "system_instruction")
	}

	// Remove model from original body if present (we put it at top level)
	delete(originalBody, "model")

	// Wrap in the Cloud Code Assist format
	wrappedBody := map[string]interface{}{
		"project": projectID,
		"model":   effectiveModel,
		"request": originalBody,
	}

	newBodyBytes, err := json.Marshal(wrappedBody)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal wrapped body: %w", err)
	}

	// Create the new request
	newReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(newBodyBytes))
	if err != nil {
		return nil, false, fmt.Errorf("failed to create new request: %w", err)
	}

	// Set headers
	newReq.Header.Set("Content-Type", "application/json")
	newReq.Header.Set("Authorization", "Bearer "+accessToken)
	for k, v := range SpoofHeaders {
		newReq.Header.Set(k, v)
	}
	if streaming {
		newReq.Header.Set("Accept", "text/event-stream")
	}

	return newReq, streaming, nil
}

// TransformResponse converts a Cloud Code Assist response back to standard Gemini format.
func TransformResponse(resp *http.Response, streaming bool) (*http.Response, error) {
	if streaming {
		return transformStreamingResponse(resp)
	}
	return transformStandardResponse(resp)
}

func transformStandardResponse(resp *http.Response) (*http.Response, error) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	var wrapped map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &wrapped); err != nil {
		// If we can't parse it, just return as-is
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return resp, nil
	}

	// Unwrap the response field if present
	if inner, ok := wrapped["response"]; ok {
		innerBytes, err := json.Marshal(inner)
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			return resp, nil
		}
		resp.Body = io.NopCloser(bytes.NewReader(innerBytes))
		resp.ContentLength = int64(len(innerBytes))
	} else {
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return resp, nil
}

func transformStreamingResponse(resp *http.Response) (*http.Response, error) {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		defer resp.Body.Close()

		buf := make([]byte, 4096)
		var lineBuffer strings.Builder

		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				lineBuffer.Write(buf[:n])

				// Process complete lines
				content := lineBuffer.String()
				lines := strings.Split(content, "\n")

				// Keep incomplete last line in buffer
				lineBuffer.Reset()
				if !strings.HasSuffix(content, "\n") && len(lines) > 0 {
					lineBuffer.WriteString(lines[len(lines)-1])
					lines = lines[:len(lines)-1]
				}

				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" {
						pw.Write([]byte("\n"))
						continue
					}

					if strings.HasPrefix(line, "data:") {
						jsonData := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
						if jsonData == "" {
							pw.Write([]byte("data:\n"))
							continue
						}

						var wrapped map[string]interface{}
						if err := json.Unmarshal([]byte(jsonData), &wrapped); err == nil {
							if inner, ok := wrapped["response"]; ok {
								innerBytes, _ := json.Marshal(inner)
								pw.Write([]byte("data: "))
								pw.Write(innerBytes)
								pw.Write([]byte("\n"))
								continue
							}
						}
					}

					// Pass through unchanged
					pw.Write([]byte(line))
					pw.Write([]byte("\n"))
				}
			}

			if err == io.EOF {
				break
			}
			if err != nil {
				return
			}
		}
	}()

	resp.Body = pr
	return resp, nil
}
