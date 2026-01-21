package proxy

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/collinshill/gemini-proxy/pkg/auth"
)

// Server is the proxy server.
type Server struct {
	creds *auth.Credentials
	port  int
}

// NewServer creates a new proxy server.
func NewServer(creds *auth.Credentials, port int) *Server {
	return &Server{
		creds: creds,
		port:  port,
	}
}

// Start starts the proxy server.
func (s *Server) Start() error {
	http.HandleFunc("/", s.handleRequest)
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("Gemini proxy listening on http://localhost%s", addr)
	log.Printf("Project: %s", s.creds.ProjectID)
	log.Printf("Point your Gemini client to: http://localhost%s/v1beta/", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Skip non-Gemini requests
	if !strings.Contains(r.URL.Path, "/models/") {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Get a valid access token (refreshing if needed)
	token, err := auth.GetValidToken(s.creds)
	if err != nil {
		log.Printf("Failed to get valid token: %v", err)
		http.Error(w, "Authentication error", http.StatusUnauthorized)
		return
	}

	// Save updated credentials if token was refreshed
	if err := auth.SaveCredentials(s.creds); err != nil {
		log.Printf("Warning: failed to save refreshed credentials: %v", err)
	}

	// Transform the request
	proxyReq, streaming, err := TransformRequest(r, s.creds.ProjectID, token)
	if err != nil {
		log.Printf("Failed to transform request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("%s %s -> %s (streaming=%v)", r.Method, r.URL.Path, proxyReq.URL.String(), streaming)

	// Make the request to Cloud Code Assist
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Proxy request failed: %v", err)
		http.Error(w, "Proxy request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Transform the response
	transformedResp, err := TransformResponse(resp, streaming)
	if err != nil {
		log.Printf("Failed to transform response: %v", err)
		http.Error(w, "Response transformation failed", http.StatusInternalServerError)
		return
	}

	// Copy response headers
	for k, vv := range transformedResp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(transformedResp.StatusCode)

	// Stream the response body
	if streaming {
		flusher, ok := w.(http.Flusher)
		if !ok {
			io.Copy(w, transformedResp.Body)
			return
		}

		buf := make([]byte, 1024)
		for {
			n, err := transformedResp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				flusher.Flush()
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Streaming error: %v", err)
				break
			}
		}
	} else {
		io.Copy(w, transformedResp.Body)
	}
}
