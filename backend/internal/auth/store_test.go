package auth

import (
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
