// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package validation

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidatePosition tests playback position validation
func TestValidatePosition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pos     float64
		wantErr bool
	}{
		{"valid zero", 0, false},
		{"valid positive", 100.5, false},
		{"valid max", 86400, false},
		{"negative", -1, true},
		{"too large", 86401, true},
		{"NaN", math.NaN(), true},
		{"Inf", math.Inf(1), true},
		{"NegInf", math.Inf(-1), true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePosition(tt.pos)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateUsername tests username validation
func TestValidateUsername(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"valid simple", "user123", false},
		{"valid with underscore", "user_name", false},
		{"valid with dash", "user-name", false},
		{"valid min length", "abc", false},
		{"valid max length", "abcdefghijklmnopqrstuvwxyz1234", false},
		{"empty", "", true},
		{"too short", "ab", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz12345", true},
		{"with spaces", "user name", true},
		{"with special chars", "user@name", true},
		{"with leading space (trimmed)", " user", false},
		{"with trailing space (trimmed)", "user ", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateUsername(tt.username)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidatePassword tests password validation
func TestValidatePassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid", "TestPass1!", false},
		{"valid with special", "TestPass1!", false},
		{"valid min length", "TestPass1!", false},
		{"empty", "", true},
		{"too short", "pass12", true},
		{"only letters", "password", true},
		{"only digits", "12345678", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePassword(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateRoomName tests room name validation
func TestValidateRoomName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		room    string
		wantErr bool
	}{
		{"valid simple", "My Room", false},
		{"valid with dash", "My-Room", false},
		{"valid with underscore", "My_Room", false},
		{"valid with numbers", "Room 123", false},
		{"valid unicode", "\u041a\u043e\u043c\u043d\u0430\u0442\u0430", false},
		{"empty", "", true},
		{"only spaces", "   ", true},
		{"too long", "this is a very long room name that exceeds fifty characters limit", true},
		{"with special chars", "Room@#$", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRoomName(tt.room)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateMagnetURI tests magnet link validation
func TestValidateMagnetURI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{"valid btih magnet", "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", false},
		{"valid with longer hash", "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef0123456789abcdef", false},
		{"empty string", "", true},
		{"plain text", "not a magnet link", true},
		{"partial magnet", "magnet:?xt=", true},
		{"magnet with short hash", "magnet:?xt=urn:btih:abc123", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateMagnetURI(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateTorrentName tests torrent name validation
func TestValidateTorrentName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		torrent string
		wantErr bool
	}{
		{"valid", "My Torrent", false},
		{"valid with special chars", "Torrent (2024) [1080p]", false},
		{"empty", "", true},
		{"only spaces", "   ", true},
		{"with control char", "Torrent\x00Name", true},
		{"with tab", "Torrent\tName", false},
		{"with newline", "Torrent\nName", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTorrentName(tt.torrent)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateFileSize tests file size validation
func TestValidateFileSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{"zero", 0, false},
		{"small", 1024, false},
		{"large", 1024 * 1024 * 1024, false},
		{"max", MaxFileSize, false},
		{"negative", -1, true},
		{"too large", MaxFileSize + 1, true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateFileSize(tt.size)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateFileIndex tests file index validation
func TestValidateFileIndex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		index    int
		maxIndex int
		wantErr  bool
	}{
		{"valid zero", 0, 10, false},
		{"valid middle", 5, 10, false},
		{"valid max", 10, 10, false},
		{"negative", -1, 10, true},
		{"exceeds max", 11, 10, true},
		{"no max check", 100, -1, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateFileIndex(tt.index, tt.maxIndex)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateTorrentID tests torrent ID validation
func TestValidateTorrentID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid hex id", "0123456789abcdef0123456789abcdef01234567", false},
		{"valid uppercase", "0123456789ABCDEF0123456789ABCDEF01234567", false},
		{"valid mixed case", "0123456789aBcDeF0123456789AbCdEf01234567", false},
		{"all zeros", "0000000000000000000000000000000000000000", false},
		{"all f's", "ffffffffffffffffffffffffffffffffffffffff", false},
		{"all F's", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", false},
		{"empty id", "", true},
		{"too short", "abc123", true},
		{"too long", "0123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"invalid chars", "xyz123456789abcdef0123456789abcdef0123456", true},
		{"special chars", "0123456789abcdef0123456789abcdef0123456!", true},
		{"with spaces", "0123456789abcdef0123456789abcdef0123456 ", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTorrentID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateRoomID tests room ID validation
func TestValidateRoomID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid hex id", "0123456789abcdef0123456789abcdef", false},
		{"valid uppercase", "0123456789ABCDEF0123456789ABCDEF", false},
		{"valid mixed case", "0123456789aBcDeF0123456789AbCdEf", false},
		{"all zeros", "00000000000000000000000000000000", false},
		{"all f's", "ffffffffffffffffffffffffffffffff", false},
		{"all F's", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", false},
		{"empty id", "", true},
		{"too short", "0123456789abcdef", true},
		{"too long", "0123456789abcdef0123456789abcdef0", true},
		{"invalid chars", "xyz123456789abcdef0123456789abcde", true},
		{"special chars", "0123456789abcdef0123456789abcdeg!", true},
		{"with spaces", "0123456789abcdef0123456789abcd ", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRoomID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
