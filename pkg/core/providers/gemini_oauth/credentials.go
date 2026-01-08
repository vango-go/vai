package gemini_oauth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ErrNoCredentials indicates that no credentials file was found.
var ErrNoCredentials = errors.New("gemini-oauth: no credentials found, run login first")

// ErrCredentialsExpired indicates that the refresh token is no longer valid.
var ErrCredentialsExpired = errors.New("gemini-oauth: credentials expired, re-run login")

// Credentials holds OAuth tokens and project information.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	ProjectID    string    `json:"project_id"`
}

// IsExpired returns true if the access token has expired.
func (c *Credentials) IsExpired() bool {
	return time.Now().After(c.Expiry)
}

// NeedsRefresh returns true if the access token needs to be refreshed.
// Returns true if the token expires within 1 minute.
func (c *Credentials) NeedsRefresh() bool {
	return time.Now().After(c.Expiry.Add(-time.Minute))
}

// CredentialsStore manages credential persistence.
type CredentialsStore interface {
	// Load retrieves credentials from storage.
	Load() (*Credentials, error)
	// Save persists credentials to storage.
	Save(creds *Credentials) error
	// Path returns the path to the credentials file.
	Path() string
}

// FileCredentialsStore implements CredentialsStore using a JSON file.
type FileCredentialsStore struct {
	path string
}

// NewFileCredentialsStore creates a file-based credentials store.
// If path is empty, uses the default path (~/.config/vango/gemini-oauth-credentials.json).
func NewFileCredentialsStore(path string) *FileCredentialsStore {
	if path == "" {
		path = DefaultCredentialsFilePath()
	}
	return &FileCredentialsStore{path: path}
}

// DefaultCredentialsFilePath returns the default credentials file path.
func DefaultCredentialsFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultCredentialsPath
	}
	return filepath.Join(home, DefaultCredentialsPath)
}

// Path returns the path to the credentials file.
func (s *FileCredentialsStore) Path() string {
	return s.path
}

// Load retrieves credentials from the JSON file.
func (s *FileCredentialsStore) Load() (*Credentials, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoCredentials
		}
		return nil, err
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

// Save persists credentials to the JSON file.
func (s *FileCredentialsStore) Save(creds *Credentials) error {
	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0600)
}

// LoadCredentials is a convenience function that loads credentials from the default path.
func LoadCredentials() (*Credentials, error) {
	store := NewFileCredentialsStore("")
	return store.Load()
}

// SaveCredentials is a convenience function that saves credentials to the default path.
func SaveCredentials(creds *Credentials) error {
	store := NewFileCredentialsStore("")
	return store.Save(creds)
}
