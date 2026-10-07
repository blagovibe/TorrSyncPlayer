package validation

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// strings30 is a username of exactly MaxUsernameLength valid characters,
// used as a boundary seed for the length checks.
var strings30 = strings.Repeat("a", MaxUsernameLength)

func FuzzValidateMagnetURI(f *testing.F) {
	seeds := []string{
		"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"magnet:?xt=urn:btih:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
		"magnet:?xt=urn:ed2k:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
		"invalid",
		"",
		"magnet:?xt=urn:btih:short",
		"magnet:?xt=urn:btih:" + string([]byte{0, 1, 2, 3, 4, 5}),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, uri string) {
		err := ValidateMagnetURI(uri)
		if uri == "" || len(uri) < 10 {
			if err == nil {
				t.Errorf("expected error for invalid URI: %q", uri)
			}
		}
	})
}

func FuzzValidateUsername(f *testing.F) {
	seeds := []string{
		"testuser",
		"ab",
		"a",
		"",
		"user@name",
		"user name",
		"user\tname",
		"verylongusernamehere123456",
		"user-name_123",
		// Regression seeds: padding used to let a name pass the length check on
		// its trimmed form while being stored with the padding intact.
		"   abc   ",
		" 0000000000000000000000000000  ",
		"\tuser\n",
		strings30,
		strings30 + "x",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, username string) {
		normalized := NormalizeUsername(username)

		// Property 1: validation must depend only on the normalized form, so a
		// padded name and its canonical form are accepted or rejected together.
		if (ValidateUsername(username) == nil) != (ValidateUsername(normalized) == nil) {
			t.Errorf("validation disagrees with its own normalized form: input=%q normalized=%q", username, normalized)
		}

		// Property 2: if the name is accepted, the value the user store will
		// persist must obey the documented limits and charset.
		if ValidateUsername(username) == nil {
			n := utf8.RuneCountInString(normalized)
			if n < MinUsernameLength || n > MaxUsernameLength {
				t.Errorf("accepted username %q normalizes to %d characters, outside [%d,%d]",
					username, n, MinUsernameLength, MaxUsernameLength)
			}
			if !usernameRegex.MatchString(normalized) {
				t.Errorf("accepted username %q normalizes to %q which fails the charset rule", username, normalized)
			}
		}
	})
}

func FuzzValidatePassword(f *testing.F) {
	seeds := []string{
		"TestPass1!",
		"short",
		"",
		"a",
		"verylongpasswordthatexceedsthemaximumallowedlengthofseventytwo",
		"pass with spaces",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, password string) {
		err := ValidatePassword(password)
		if err == nil {
			if len(password) < 8 || len(password) > 72 {
				t.Errorf("expected error for password length %d", len(password))
			}
		}
	})
}

func FuzzValidateFileIndex(f *testing.F) {
	seeds := []int{0, 1, -1, 5, 100}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, index int) {
		_ = ValidateFileIndex(index, 10)
	})
}
