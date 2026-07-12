package validation

import (
	"strings"
	"testing"
)

// FuzzValidateRoomName tests room name validation
func FuzzValidateRoomName(f *testing.F) {
	// Valid room names
	f.Add("My Room")
	f.Add("Room123")
	f.Add("Русская комната")
	f.Add("部屋")
	f.Add("Room_with_underscores")
	f.Add("Room-with-dashes")

	// Invalid room names (should not panic)
	f.Add("")
	f.Add(strings.Repeat("a", 1000))
	f.Add("Room<script>alert(1)</script>")
	f.Add("Room\x00null")
	f.Add("Room\nnewline")

	f.Fuzz(func(t *testing.T, data string) {
		err := ValidateRoomName(data)
		_ = err // Just ensure no panic
	})
}

// FuzzValidateTorrentID tests torrent ID validation
func FuzzValidateTorrentID(f *testing.F) {
	// Valid torrent IDs (40 hex chars)
	f.Add("abcdef1234567890abcdef1234567890abcdef12")
	f.Add("ABCDEF1234567890ABCDEF1234567890ABCDEF12")
	f.Add("0000000000000000000000000000000000000000")
	f.Add("ffffffffffffffffffffffffffffffffffffffff")

	// Invalid torrent IDs
	f.Add("")
	f.Add("abc")
	f.Add("ghijklmnopqrstuvwxyz1234567890abcdef12") // non-hex
	f.Add(strings.Repeat("a", 39))                    // too short
	f.Add(strings.Repeat("a", 41))                    // too long
	f.Add("abcdef1234567890abcdef1234567890abcdef12\n") // with newline

	f.Fuzz(func(t *testing.T, data string) {
		err := ValidateTorrentID(data)
		_ = err
	})
}

// FuzzValidateFileIndex tests file index validation
func FuzzValidateFileIndex(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(99)
	f.Add(1000)
	f.Add(-1)
	f.Add(-100)
	f.Add(2147483647) // max int32

	f.Fuzz(func(t *testing.T, data int) {
		err := ValidateFileIndex(data, 100)
		_ = err
	})
}

// FuzzValidatePosition tests playback position validation
func FuzzValidatePosition(f *testing.F) {
	f.Add(0.0)
	f.Add(1.5)
	f.Add(3600.0) // 1 hour
	f.Add(7200.0) // 2 hours
	f.Add(-1.0)
	f.Add(-100.0)
	f.Add(1e10) // very large

	f.Fuzz(func(t *testing.T, data float64) {
		err := ValidatePosition(data)
		_ = err
	})
}

// FuzzValidatePassword tests password validation
func FuzzValidatePassword(f *testing.F) {
	// Valid passwords
	f.Add("Password123!")
	f.Add("P@ssw0rd!")
	f.Add("VeryLongPassword12345678901234567890")
	f.Add("Пароль1234!")
	f.Add("パスワード1234!")

	// Invalid passwords
	f.Add("")
	f.Add("123") // too short
	f.Add(strings.Repeat("a", 129))
	f.Add("password\n") // newline
	f.Add("pass\x00word") // null byte

	f.Fuzz(func(t *testing.T, data string) {
		err := ValidatePassword(data)
		_ = err
	})
}

// FuzzSanitizeString tests string sanitization
func FuzzSanitizeString(f *testing.F) {
	f.Add("normal text")
	f.Add("text with <script>alert(1)</script>")
	f.Add("text\x00with\x01control\x02chars")
	f.Add("text\nwith\r\nnewlines")
	f.Add(strings.Repeat("a", 10000))
	f.Add("")

	f.Fuzz(func(t *testing.T, data string) {
		result := SanitizeString(data)

		// Should not contain control characters except newline/tab
		for _, r := range result {
			if r < 32 && r != '\n' && r != '\r' && r != '\t' {
				t.Errorf("Sanitized input contains control char: %d", r)
			}
		}

		// Should not be longer than input (sanitization removes, doesn't add)
		if len(result) > len(data) {
			t.Errorf("Sanitized output longer than input: %d > %d", len(result), len(data))
		}
	})
}