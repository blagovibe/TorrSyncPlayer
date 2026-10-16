// SPDX-License-Identifier: MIT
package validation

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
)

// Boundary branches the table tests never touch. Each case below kills a
// condition mutant: drop the condition and exactly this case flips.

// TestValidatePasswordBranches covers every rejection branch one by one. The
// table test only proves short/empty/letters-only/digits-only, so a mutant
// deleting any single complexity check survives it.
func TestValidatePasswordBranches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid", "TestPass1!", false},
		{"no uppercase", "testpass1!", true},
		{"no lowercase", "TESTPASS1!", true},
		{"no digit", "TestPass!!", true},
		{"no special", "TestPass12", true},
		{"common password", "password123", true},
		{"common password case-insensitive", "PASSWORD123", true},
		{"too long", "TestPass1!" + strings.Repeat("x", MaxPasswordLength), true},
		{"exactly max length valid", "Aa1!" + strings.Repeat("x", MaxPasswordLength-4), false},
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

// TestNormalizeUsername pins the canonical form directly: trim, then
// lowercase. Validation leans on it, so a mutant dropping either half must
// fail here rather than hide behind end-to-end cases.
func TestNormalizeUsername(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "alice", NormalizeUsername("  Alice  "))
	assert.Equal(t, "alice", NormalizeUsername("ALICE"))
	assert.Equal(t, "", NormalizeUsername("   "))
	assert.Equal(t, "bob", NormalizeUsername("bob"))
}

// TestValidateMagnetURIEdges covers length and parameter branches.
func TestValidateMagnetURIEdges(t *testing.T) {
	t.Parallel()
	good := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"

	assert.NoError(t, ValidateMagnetURI(good+"&dn=film"))
	assert.NoError(t, ValidateMagnetURI(good+"&&dn=film"),
		"empty parameter segments are skipped, not rejected")

	assert.Error(t, ValidateMagnetURI(good+strings.Repeat("a", 2048)),
		"over the length limit must fail")
	assert.Error(t, ValidateMagnetURI(good+"&dn=fi lm"),
		"a space is not a legal parameter character")
}

// TestValidateTorrentNameEdges covers the length boundary in runes (not
// bytes) and the remaining control characters.
func TestValidateTorrentNameEdges(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ValidateTorrentName(strings.Repeat("ü", 255)),
		"the limit counts runes, and 255 multibyte runes fit")
	assert.Error(t, ValidateTorrentName(strings.Repeat("a", 256)))
	assert.Error(t, ValidateTorrentName("film\x1fname"))
	assert.NoError(t, ValidateTorrentName("r\t\n"))
}

// TestValidateFileIndexEdges covers the max boundary and a negative max
// other than the -1 "no check" sentinel.
func TestValidateFileIndexEdges(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ValidateFileIndex(0, 0))
	assert.Error(t, ValidateFileIndex(1, 0))
	assert.NoError(t, ValidateFileIndex(100, -2),
		"any negative max disables the range check, not just -1")
}

// TestValidateTorrentFile covers the base64 entry point end to end.
func TestValidateTorrentFile(t *testing.T) {
	t.Parallel()
	assert.Error(t, ValidateTorrentFile(""))
	assert.Error(t, ValidateTorrentFile("!!! not base64 !!!"))

	// 100 decodable bytes: over the 50-byte minimum, under the 1MB maximum.
	blob := strings.Repeat("A", 136) // 136 base64 chars -> 102 bytes
	assert.NoError(t, ValidateTorrentFile(blob))
	assert.Error(t, ValidateTorrentFile(strings.Repeat("A", 8)),
		"6 bytes decode under the minimum")

	huge := strings.Repeat("A", (int(constants.MaxTorrentFileSize)+3)/3*4)
	assert.Error(t, ValidateTorrentFile(huge))
}

// TestDecodeTorrentFile pins the reader path separately from validation.
func TestDecodeTorrentFile(t *testing.T) {
	t.Parallel()
	_, err := DecodeTorrentFile("!!! not base64 !!!")
	assert.Error(t, err)

	r, err := DecodeTorrentFile(strings.Repeat("A", 136))
	require.NoError(t, err)
	assert.NotNil(t, r)
}

// TestErrorMessagesPinBranches pins which branch rejected the input. A test
// asserting only "an error" lets a mutant delete one check while another
// catches the same input for a different reason — green suite, dead check.
func TestErrorMessagesPinBranches(t *testing.T) {
	t.Parallel()

	err := ValidatePassword("Password1!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too common",
		"a blacklisted password must fail on the blacklist, not on complexity")

	assert.EqualError(t, ValidateTorrentID(""), "torrent ID cannot be empty")
	assert.EqualError(t, ValidateTorrentFile(""), "torrent file cannot be empty")
	assert.EqualError(t, ValidateDecodedTorrentSize(0), "torrent file size must be positive")

	require.Error(t, ValidatePosition(math.NaN()))
	assert.Contains(t, ValidatePosition(math.NaN()).Error(), "NaN or Inf")
}

// TestValidateDecodedTorrentSize covers every boundary of the size ladder.
func TestValidateDecodedTorrentSize(t *testing.T) {
	t.Parallel()
	assert.Error(t, ValidateDecodedTorrentSize(0))
	assert.Error(t, ValidateDecodedTorrentSize(-5))
	assert.Error(t, ValidateDecodedTorrentSize(49))
	assert.NoError(t, ValidateDecodedTorrentSize(50))
	assert.NoError(t, ValidateDecodedTorrentSize(constants.MaxTorrentFileSize))
	assert.Error(t, ValidateDecodedTorrentSize(constants.MaxTorrentFileSize+1))
}
