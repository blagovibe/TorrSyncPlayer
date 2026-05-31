package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ==================== SanitizeLogValue Tests ====================

func TestSanitizeLogValue_MagnetURI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "magnet with multiple params",
			input:    "magnet:?xt=urn:btih:abc123&dn=video&tr=tracker",
			expected: "magnet:?xt=urn:btih:abc123&...",
		},
		{
			name:     "magnet with only xt param",
			input:    "magnet:?xt=urn:btih:abc123",
			expected: "magnet:?xt=urn:btih:abc123&...",
		},
		{
			name:     "magnet with empty params",
			input:    "magnet:?xt=urn:btih:abc123&",
			expected: "magnet:?xt=urn:btih:abc123&...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeLogValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeLogValue_NonMagnetValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "regular string",
			input:    "some regular log value",
			expected: "some regular log value",
		},
		{
			name:     "file path",
			input:    "/path/to/file.torrent",
			expected: "/path/to/file.torrent",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:  "string starting with magnet but no question mark",
			input: "magnet:something",
			// Note: strings.HasPrefix("magnet:something", "magnet:") is true
			// So it will be treated as magnet URI and get "&..." appended
			expected: "magnet:something&...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeLogValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeLogValue_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "value with newlines",
			input:    "value\nwith\nnewlines",
			expected: "value\nwith\nnewlines",
		},
		{
			name:     "value with tabs",
			input:    "value\twith\ttabs",
			expected: "value\twith\ttabs",
		},
		{
			name:     "value with unicode",
			input:    "значение на русском",
			expected: "значение на русском",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeLogValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ==================== ValidateFilePath Tests ====================

func TestValidateFilePath_ValidPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple torrent file",
			input: "video.torrent",
		},
		{
			name:  "torrent in subdirectory",
			input: "downloads/video.torrent",
		},
		{
			name:  "absolute path to torrent",
			input: "/home/user/torrents/video.torrent",
		},
		{
			name:  "windows path to torrent",
			input: "C:\\Users\\user\\torrents\\video.torrent",
		},
		{
			name:  "uppercase extension",
			input: "file.TORRENT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilePath(tt.input)
			assert.NoError(t, err)
		})
	}
}

func TestValidateFilePath_InvalidPaths(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedErr string
	}{
		{
			name:        "empty path",
			input:       "",
			expectedErr: "empty file path",
		},
		{
			name:        "path with null bytes",
			input:       "file\x00.torrent",
			expectedErr: "file path contains null bytes",
		},
		{
			name:        "path traversal with double dots",
			input:       "../../../etc/passwd.torrent",
			expectedErr: "file path contains invalid characters",
		},
		{
			name:        "path traversal in middle",
			input:       "downloads/../../../etc/passwd.torrent",
			expectedErr: "file path contains invalid characters",
		},
		{
			name:        "wrong extension - .exe",
			input:       "malware.exe",
			expectedErr: "invalid file extension: expected .torrent",
		},
		{
			name:        "wrong extension - .txt",
			input:       "readme.txt",
			expectedErr: "invalid file extension: expected .torrent",
		},
		{
			name:        "no extension",
			input:       "filewithoutextension",
			expectedErr: "invalid file extension: expected .torrent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilePath(tt.input)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestValidateFilePath_LongPath(t *testing.T) {
	// Create a path longer than 4096 characters
	longPath := strings.Repeat("a", 4097) + ".torrent"
	err := validateFilePath(longPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "file path too long")
}

func TestValidateFilePath_MaxLengthPath(t *testing.T) {
	// Create a path exactly 4096 characters (should pass length check)
	basePath := strings.Repeat("a", 4096-len(".torrent"))
	validPath := basePath + ".torrent"
	err := validateFilePath(validPath)
	// Should pass length check but may fail on other validations
	// The important thing is it doesn't fail on length
	if err != nil {
		assert.NotContains(t, err.Error(), "file path too long")
	}
}

// ==================== SanitizePath Tests ====================

func TestSanitizePath_BasicSanitization(t *testing.T) {
	sep := string(filepath.Separator)
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes null bytes",
			input:    "path\x00/to\x00/file",
			expected: "path" + sep + "to" + sep + "file",
		},
		{
			name:     "cleans path",
			input:    "path/../path/./file",
			expected: "path" + sep + "file",
		},
		{
			name:     "removes leading separator",
			input:    "/absolute/path",
			expected: "absolute" + sep + "path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ==================== LogSecurityEvent Tests ====================

func TestLogSecurityEvent(t *testing.T) {
	// This test just ensures the function doesn't panic
	details := map[string]interface{}{
		"ip":     "192.168.1.1",
		"action": "login_attempt",
	}

	// Should not panic
	assert.NotPanics(t, func() {
		LogSecurityEvent("auth_attempt", details)
	})
}

// ==================== Bcrypt Password Tests ====================

// HashPassword хеширует пароль с использованием bcrypt
func HashPassword(password string) (string, error) {
	// bcrypt has a 72 byte limit, truncate if necessary
	if len(password) > 72 {
		password = password[:72]
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword проверяет пароль против хеша
func CheckPassword(password, hash string) bool {
	// bcrypt has a 72 byte limit, truncate if necessary
	if len(password) > 72 {
		password = password[:72]
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{
			name:     "simple password",
			password: "password123",
		},
		{
			name:     "complex password",
			password: "P@$$w0rd!#$%^&*()",
		},
		{
			name:     "unicode password",
			password: "парольнарусском",
		},
		{
			name:     "long password (truncated to 72 bytes)",
			password: strings.Repeat("a", 100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			require.NoError(t, err)
			assert.NotEmpty(t, hash)
			assert.NotEqual(t, tt.password, hash)

			// Verify hash is valid bcrypt
			assert.True(t, strings.HasPrefix(hash, "$2a$"))
		})
	}
}

func TestCheckPassword_CorrectPassword(t *testing.T) {
	password := "testpassword123"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	assert.True(t, CheckPassword(password, hash))
}

func TestCheckPassword_IncorrectPassword(t *testing.T) {
	password := "testpassword123"
	wrongPassword := "wrongpassword"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	assert.False(t, CheckPassword(wrongPassword, hash))
}

func TestCheckPassword_EmptyPassword(t *testing.T) {
	password := "testpassword123"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	assert.False(t, CheckPassword("", hash))
}

func TestCheckPassword_InvalidHash(t *testing.T) {
	assert.False(t, CheckPassword("password", "invalidhash"))
}

func TestHashPassword_UniqueHashes(t *testing.T) {
	// Same password should produce different hashes (due to salt)
	password := "samepassword"
	hash1, err := HashPassword(password)
	require.NoError(t, err)
	hash2, err := HashPassword(password)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2)

	// But both should verify correctly
	assert.True(t, CheckPassword(password, hash1))
	assert.True(t, CheckPassword(password, hash2))
}

// ==================== Integration Tests ====================

func TestSecurityIntegration_PathValidationAndSanitization(t *testing.T) {
	// Test that validation catches malicious paths before sanitization
	maliciousPaths := []string{
		"../../../etc/passwd\x00.torrent",
		"..\\..\\windows\\system32\\config.torrent",
		".\x00./.\x00./etc/shadow.torrent",
	}

	for _, path := range maliciousPaths {
		err := validateFilePath(path)
		assert.Error(t, err, "Expected error for malicious path: %s", path)
	}
}

func TestSecurityIntegration_LogSanitizationWithMagnet(t *testing.T) {
	// Test that sensitive magnet URI params are not logged
	magnet := "magnet:?xt=urn:btih:abc123&dn=secret_video&tr=http://private-tracker.com/announce&passkey=secret123"
	sanitized := SanitizeLogValue(magnet)

	// Should only contain the xt param, not dn, tr, or passkey
	assert.Contains(t, sanitized, "xt=urn:btih:abc123")
	assert.NotContains(t, sanitized, "secret_video")
	assert.NotContains(t, sanitized, "private-tracker")
	assert.NotContains(t, sanitized, "passkey")
	assert.NotContains(t, sanitized, "secret123")
}
