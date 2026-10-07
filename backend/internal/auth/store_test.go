package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/utils"
)

func TestUserStoreCreate(t *testing.T) {
	store := NewUserStore()

	tests := []struct {
		name        string
		username    string
		password    string
		expectError bool
	}{
		{
			name:        "Valid user",
			username:    "testuser",
			password:    "TestPass1!",
			expectError: false,
		},
		{
			name:        "Empty username",
			username:    "",
			password:    "TestPass1!",
			expectError: true,
		},
		{
			name:        "Short username",
			username:    "ab",
			password:    "TestPass1!",
			expectError: true,
		},
		{
			name:        "Long username",
			username:    "abcdefghijklmnopqrstuvwxyz1234567890",
			password:    "TestPass1!",
			expectError: true,
		},
		{
			name:        "Empty password",
			username:    "testuser2",
			password:    "",
			expectError: true,
		},
		{
			name:        "Short password (less than 8 characters)",
			username:    "testuser3",
			password:    "12345",
			expectError: true,
		},
		{
			name:        "Password without letters",
			username:    "testuser4",
			password:    "12345678",
			expectError: true,
		},
		{
			name:        "Password without digits",
			username:    "testuser5",
			password:    "password",
			expectError: true,
		},
		{
			name:        "Name with invalid characters",
			username:    "test@user!",
			password:    "TestPass1!",
			expectError: true,
		},
		{
			name:        "Valid user with hyphen",
			username:    "test-user",
			password:    "TestPass1!",
			expectError: false,
		},
		{
			name:        "Valid user with underscore",
			username:    "test_user",
			password:    "TestPass1!",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := store.Create(tt.username, tt.password)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, user)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, user)
			assert.Equal(t, tt.username, user.Username)
			assert.NotEmpty(t, user.ID)
			assert.NotEmpty(t, user.PasswordHash)
			assert.NotEqual(t, tt.password, user.PasswordHash) // Password must be hashed
		})
	}
}

func TestUserStoreCreateDuplicate(t *testing.T) {
	store := NewUserStore()

	// Create first user
	_, err := store.Create("testuser", "TestPass1!")
	require.NoError(t, err)

	// Attempt to create duplicate
	_, err = store.Create("testuser", "TestPass2!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestUserStoreAuthenticate(t *testing.T) {
	store := NewUserStore()

	// Create user
	_, err := store.Create("testuser", "TestPass1!")
	require.NoError(t, err)

	tests := []struct {
		name        string
		username    string
		password    string
		expectError bool
	}{
		{
			name:        "Correct credentials",
			username:    "testuser",
			password:    "TestPass1!",
			expectError: false,
		},
		{
			name:        "Wrong password",
			username:    "testuser",
			password:    "wrongpassword",
			expectError: true,
		},
		{
			name:        "Non-existent user",
			username:    "nonexistent",
			password:    "TestPass1!",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := store.Authenticate(tt.username, tt.password)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.username, user.Username)
			}
		})
	}
}

func TestUserStoreGetByUsername(t *testing.T) {
	store := NewUserStore()

	// Create user
	created, err := store.Create("testuser", "TestPass1!")
	require.NoError(t, err)

	// Get by username
	user, exists := store.GetByUsername("testuser")
	assert.True(t, exists)
	assert.Equal(t, created.ID, user.ID)

	// Non-existent user
	_, exists = store.GetByUsername("nonexistent")
	assert.False(t, exists)
}

// TestUserStoreUsernameNormalization pins the invariant that the form used for
// validation is the same form used for storage and lookup.
//
// Validation used to TrimSpace internally while the store only lowercased, so
// "  Alice  " was checked as "Alice" (5 chars, accepted) but stored as
// "  alice  ". That record could then never be found by a login for "alice"
// or "ALICE", and the 30-character limit did not bound the stored value.
func TestUserStoreUsernameNormalization(t *testing.T) {
	store := NewUserStore()

	created, err := store.Create("  Alice  ", "TestPass1!")
	require.NoError(t, err)

	// Stored under the canonical form, not the padded input.
	assert.Equal(t, "alice", created.Username)

	// Every spelling of the same logical name resolves to the same account.
	for _, spelling := range []string{"alice", "ALICE", "  Alice  ", "\talice\n", "aLiCe"} {
		user, exists := store.GetByUsername(spelling)
		assert.True(t, exists, "lookup failed for %q", spelling)
		require.NotNil(t, user)
		assert.Equal(t, created.ID, user.ID, "wrong user for %q", spelling)

		authenticated, err := store.Authenticate(spelling, "TestPass1!")
		assert.NoError(t, err, "authenticate failed for %q", spelling)
		require.NotNil(t, authenticated)
		assert.Equal(t, created.ID, authenticated.ID)
	}

	// A padded name that normalises to exactly MaxUsernameLength is accepted,
	// and the stored value respects the limit.
	padded := "  " + strings.Repeat("b", 30) + "  "
	atLimit, err := store.Create(padded, "TestPass1!")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("b", 30), atLimit.Username)
	assert.Len(t, atLimit.Username, 30)

	// One character over the limit is still rejected despite the padding.
	tooLong := "  " + strings.Repeat("c", 31) + "  "
	_, err = store.Create(tooLong, "TestPass1!")
	assert.Error(t, err)
}

func TestUserStoreGetByID(t *testing.T) {
	store := NewUserStore()

	// Create user
	created, err := store.Create("testuser", "TestPass1!")
	require.NoError(t, err)

	// Get by ID
	user, exists := store.GetByID(created.ID)
	assert.True(t, exists)
	assert.Equal(t, "testuser", user.Username)

	// Non-existent ID
	_, exists = store.GetByID("nonexistent-id")
	assert.False(t, exists)
}

func TestGenerateID(t *testing.T) {
	// Generate several IDs and check uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := utils.GenerateID(16)
		assert.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.False(t, ids[id], "ID must be unique")
		ids[id] = true
	}
}

func TestUserStoreChangePassword(t *testing.T) {
	store := NewUserStore()

	// Create user
	_, err := store.Create("testuser", "TestPass1!")
	require.NoError(t, err)

	tests := []struct {
		name            string
		username        string
		currentPassword string
		newPassword     string
		expectError     bool
	}{
		{
			name:            "Valid password change",
			username:        "testuser",
			currentPassword: "TestPass1!",
			newPassword:     "NewPass1!",
			expectError:     false,
		},
		{
			name:            "Wrong current password",
			username:        "testuser",
			currentPassword: "WrongPass1!",
			newPassword:     "NewPass1!",
			expectError:     true,
		},
		{
			name:            "Non-existent user",
			username:        "nonexistent",
			currentPassword: "TestPass1!",
			newPassword:     "NewPass1!",
			expectError:     true,
		},
		{
			name:            "Invalid new password (too short)",
			username:        "testuser",
			currentPassword: "TestPass1!",
			newPassword:     "short",
			expectError:     true,
		},
		{
			name:            "Invalid new password (common password)",
			username:        "testuser",
			currentPassword: "TestPass1!",
			newPassword:     "password",
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			switch tt.name {
			case "Non-existent user":
				err = store.ChangePassword(tt.username, tt.currentPassword, tt.newPassword)
			case "Invalid new password (too short)", "Invalid new password (common password)":
				// For these tests, user should already exist from "Valid password change" test
				// Just try to change password
				err = store.ChangePassword("testuser", "TestPass1!", tt.newPassword)
				// Reset password back for other tests
				if err != nil {
					_ = store.ChangePassword("testuser", tt.newPassword, "TestPass1!")
				}
			default:
				err = store.ChangePassword(tt.username, tt.currentPassword, tt.newPassword)
			}

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify new password works
				user, authErr := store.Authenticate(tt.username, tt.newPassword)
				require.NoError(t, authErr)
				assert.NotNil(t, user)
			}
		})
	}

	// Test case-insensitive username
	t.Run("Case insensitive username", func(t *testing.T) {
		_, err := store.Create("caseuser", "TestPass1!")
		require.NoError(t, err)

		// Change password with different case
		err = store.ChangePassword("CASEUSER", "TestPass1!", "NewPass1!")
		require.NoError(t, err)

		// Verify old password doesn't work
		_, err = store.Authenticate("caseuser", "TestPass1!")
		assert.Error(t, err)

		// Verify new password works
		user, err := store.Authenticate("caseuser", "NewPass1!")
		require.NoError(t, err)
		assert.NotNil(t, user)
	})
}
