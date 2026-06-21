package validation

import (
	"testing"
)

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
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, username string) {
		err := ValidateUsername(username)
		if err == nil {
			if len(username) < 3 || len(username) > 30 {
				t.Errorf("expected error for username length %d: %q", len(username), username)
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
