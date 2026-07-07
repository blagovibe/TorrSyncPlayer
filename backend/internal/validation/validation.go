// SPDX-License-Identifier: MIT

// Package validation provides common validation functions.
// Contains utilities for data correctness checks.
package validation

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
)

// Regular expressions for validation
var (
	// roomNameRegex allows letters, digits, spaces, hyphens and underscores
	roomNameRegex = regexp.MustCompile(`^[\p{L}\p{N}\s\-_]{1,50}$`)
	// usernameRegex allows letters, digits, underscores and hyphens
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-]{3,30}$`)
	// magnetRegex for validating magnet links - restricted to btih URN type only
	// This provides security by limiting to BitTorrent infohash URNs and preventing SSRF
	magnetRegex      = regexp.MustCompile(`^magnet:\?xt=urn:btih:[a-fA-F0-9]{40}`)
	magnetParamsRegex  = regexp.MustCompile(`^[a-zA-Z0-9\-._~%!$&'()*+,;=:@/?]+$`)
)

// Common passwords blacklist - passwords that should never be allowed
var commonPasswords = map[string]bool{
	"password":    true,
	"password1":   true,
	"password123": true,
	"123456":      true,
	"12345678":    true,
	"qwerty":      true,
	"abc123":      true,
	"monkey":      true,
	"master":      true,
	"dragon":      true,
	"letmein":     true,
	"login":       true,
	"admin":       true,
	"welcome":     true,
	"football":    true,
	"iloveyou":    true,
	"starwars":    true,
	"batman":      true,
	"trustno1":    true,
	"password1!":  true,
}

// Size limits
const (
	MaxUsernameLength    = 30
	MinUsernameLength    = 3
	MinPasswordLength    = 8
	MaxRoomNameLength    = 50
	MinRoomNameLength    = 1
	MaxTorrentNameLength = 255
	MaxFileSize          = constants.MaxStreamFileSize
)

const (
	MaxPasswordLength = constants.MaxPasswordLength
	MaxRequestSize    = constants.MaxRequestSize
)

// ── Validation Constants ──────────────────────────────────────────────────

const (
	// MaxStringLength maximum length for sanitized strings
	MaxStringLength = 1000

	// MaxPositionSeconds maximum playback position in seconds (24 hours)
	MaxPositionSeconds = 86400
)

var ErrInvalidPosition = errors.New("invalid playback position")

// ValidatePosition validates the playback position.
// Checks that position is not NaN, Inf, negative and does not exceed 24 hours.
// Returns an error if the position is invalid.
func ValidatePosition(position float64) error {
	if math.IsNaN(position) || math.IsInf(position, 0) {
		return fmt.Errorf("%w: NaN or Inf", ErrInvalidPosition)
	}
	if position < 0 {
		return fmt.Errorf("%w: negative value %f", ErrInvalidPosition, position)
	}
	if position > MaxPositionSeconds {
		return fmt.Errorf("%w: exceeds 24 hours: %f", ErrInvalidPosition, position)
	}
	return nil
}

// ValidateUsername validates the username.
// Checks length, allowed characters and absence of leading/trailing spaces.
func ValidateUsername(username string) error {
	username = strings.TrimSpace(username)

	usernameLen := utf8.RuneCountInString(username)
	if usernameLen < MinUsernameLength || usernameLen > MaxUsernameLength {
		return fmt.Errorf("username must be between %d and %d characters", MinUsernameLength, MaxUsernameLength)
	}

	if !usernameRegex.MatchString(username) {
		return fmt.Errorf("username can only contain letters, numbers, underscores, and hyphens")
	}

	return nil
}

// ValidatePassword validates the password.
// Checks length, complexity (presence of upper, lower, digit, special), and common password blacklist.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("password too short (minimum %d characters)", MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("password too long (maximum %d bytes)", MaxPasswordLength)
	}

	// Check against common passwords blacklist (case-insensitive)
	lowerPassword := strings.ToLower(password)
	if commonPasswords[lowerPassword] {
		return fmt.Errorf("password is too common, please choose a stronger password")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}

// ValidateRoomName validates the room name.
// Checks length and allowed characters.
func ValidateRoomName(name string) error {
	name = strings.TrimSpace(name)

	if utf8.RuneCountInString(name) < MinRoomNameLength {
		return fmt.Errorf("room name cannot be empty")
	}
	if utf8.RuneCountInString(name) > MaxRoomNameLength {
		return fmt.Errorf("room name too long (maximum %d characters)", MaxRoomNameLength)
	}
	if !roomNameRegex.MatchString(name) {
		return fmt.Errorf("room name contains invalid characters")
	}
	return nil
}

// ValidateMagnetURI validates the magnet link format.
// Ensures the link uses btih URN (not other schemes) and validates parameter format.
func ValidateMagnetURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("magnet link cannot be empty")
	}
	if len(uri) > 2048 {
		return fmt.Errorf("magnet link too long")
	}
	if !magnetRegex.MatchString(uri) {
		return fmt.Errorf("invalid magnet link format: must start with 'magnet:?xt=urn:btih:' followed by 40 hex characters")
	}

	// Extract and validate parameters (after xt=urn:btih)
	paramsPart := ""
	for i, c := range uri {
		if i >= 19 && c == '&' {
			paramsPart = uri[20:]
			break
		}
		if i >= 19 {
			paramsPart = uri[20:]
			break
		}
	}

	// Allow empty params or validate each param
	if paramsPart != "" {
		// Split by & and validate each parameter
		params := strings.Split(paramsPart, "&")
		for _, param := range params {
			if param == "" {
				continue
			}
			// Check for dangerous characters that could be used for injection
			if strings.ContainsAny(param, "<>\"'{}|\\^[]`") {
				return fmt.Errorf("magnet link contains invalid characters in parameters")
			}
		}
	}

	return nil
}

// ValidateTorrentName validates the torrent name.
func ValidateTorrentName(name string) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("torrent name cannot be empty")
	}
	if utf8.RuneCountInString(name) > MaxTorrentNameLength {
		return fmt.Errorf("torrent name too long (maximum %d characters)", MaxTorrentNameLength)
	}

	// Check for control characters
	for _, r := range name {
		if r < 32 && r != '\t' && r != '\n' {
			return fmt.Errorf("torrent name contains invalid characters")
		}
	}

	return nil
}

// ValidateFileSize validates the file size.
func ValidateFileSize(size int64) error {
	if size < 0 {
		return fmt.Errorf("file size cannot be negative")
	}
	if size > MaxFileSize {
		return fmt.Errorf("file too large (maximum %d GB)", MaxFileSize/(1024*1024*1024))
	}
	return nil
}

// ValidateFileIndex validates the file index.
func ValidateFileIndex(index, maxIndex int) error {
	if index < 0 {
		return fmt.Errorf("file index cannot be negative")
	}
	if maxIndex >= 0 && index > maxIndex {
		return fmt.Errorf("file index out of range")
	}
	return nil
}

// ValidateTorrentID validates the torrent ID (hex string of 40 characters).
func ValidateTorrentID(id string) error {
	if id == "" {
		return fmt.Errorf("torrent ID cannot be empty")
	}
	if len(id) != 40 {
		return fmt.Errorf("torrent ID must be 40 characters (got %d)", len(id))
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return fmt.Errorf("torrent ID contains invalid character: %c", c)
		}
	}
	return nil
}

// ValidateRoomID validates the room ID (hex string of 32 characters = 16 bytes).
func ValidateRoomID(id string) error {
	if id == "" {
		return fmt.Errorf("room ID cannot be empty")
	}
	if len(id) != 32 {
		return fmt.Errorf("room ID must be 32 characters (got %d)", len(id))
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return fmt.Errorf("room ID contains invalid character: %c", c)
		}
	}
	return nil
}

// SanitizeString cleans a string from potentially dangerous characters.
// Used for displaying user input.
func SanitizeString(s string) string {
	// Remove control characters
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' && r != '\n' {
			return -1
		}
		return r
	}, s)

	// Limit length
	if utf8.RuneCountInString(s) > MaxStringLength {
		s = string([]rune(s)[:MaxStringLength])
	}

	return strings.TrimSpace(s)
}
