package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewP2PService(t *testing.T) {
	service := NewP2PService()
	require.NotNil(t, service, "P2PService should not be nil")
	assert.NotNil(t, service.peers, "peers map should be initialized")
	assert.NotNil(t, service.heartbeatStopCh, "heartbeatStopCh map should be initialized")
	assert.NotEmpty(t, service.localPeerID, "localPeerID should be generated")
	assert.Equal(t, 2*1000000000, int(service.heartbeatInterval), "heartbeat interval should be 2 seconds")
}

func TestGenerateRoomID(t *testing.T) {
	// Test that roomID is generated
	roomID := generateRoomID()
	assert.NotEmpty(t, roomID, "roomID should not be empty")

	// RoomID should be 16 hex characters (8 bytes * 2 hex chars)
	assert.Equal(t, 16, len(roomID), "roomID should be 16 characters long")

	// Should be valid hex
	for _, c := range roomID {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"roomID should contain only hex characters, got: %c", c)
	}
}

func TestGenerateRoomIDUniqueness(t *testing.T) {
	// Generate multiple roomIDs and check they are unique
	roomIDs := make(map[string]bool)
	for i := 0; i < 100; i++ {
		roomID := generateRoomID()
		assert.False(t, roomIDs[roomID], "roomID should be unique, duplicate found: %s", roomID)
		roomIDs[roomID] = true
	}
}

func TestGeneratePeerID(t *testing.T) {
	peerID := generatePeerID()
	assert.NotEmpty(t, peerID, "peerID should not be empty")

	// PeerID should be 16 hex characters (8 bytes * 2 hex chars)
	assert.Equal(t, 16, len(peerID), "peerID should be 16 characters long")

	// Should be valid hex
	for _, c := range peerID {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"peerID should contain only hex characters, got: %c", c)
	}
}

func TestGeneratePeerIDUniqueness(t *testing.T) {
	// Generate multiple peerIDs and check they are unique
	peerIDs := make(map[string]bool)
	for i := 0; i < 100; i++ {
		peerID := generatePeerID()
		assert.False(t, peerIDs[peerID], "peerID should be unique, duplicate found: %s", peerID)
		peerIDs[peerID] = true
	}
}

func TestP2PMessageSerialization(t *testing.T) {
	tests := []struct {
		name    string
		message P2PMessage
	}{
		{
			name: "play message",
			message: P2PMessage{
				Type:      MsgPlay,
				Timestamp: 1234567890,
				Data: map[string]interface{}{
					"position": 100.5,
				},
			},
		},
		{
			name: "pause message",
			message: P2PMessage{
				Type:      MsgPause,
				Timestamp: 1234567890,
				Data: map[string]interface{}{
					"position": 200.0,
				},
			},
		},
		{
			name: "seek message",
			message: P2PMessage{
				Type:      MsgSeek,
				Timestamp: 1234567890,
				Data: map[string]interface{}{
					"position": 300.0,
				},
			},
		},
		{
			name: "heartbeat message",
			message: P2PMessage{
				Type:      MsgHeartbeat,
				Timestamp: 1234567890,
				Data:      nil,
			},
		},
		{
			name: "state message",
			message: P2PMessage{
				Type:      MsgState,
				Timestamp: 1234567890,
				Data: map[string]interface{}{
					"isPlaying": true,
					"position":  150.0,
				},
			},
		},
		{
			name: "chat message",
			message: P2PMessage{
				Type:      MsgChat,
				Timestamp: 1234567890,
				Data: map[string]interface{}{
					"text": "Hello!",
					"from": "peer123",
				},
			},
		},
		{
			name: "torrent info message",
			message: P2PMessage{
				Type:      MsgTorrentInfo,
				Timestamp: 1234567890,
				Data: map[string]interface{}{
					"hash": "abc123",
					"name": "Test Torrent",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize
			data, err := json.Marshal(tt.message)
			require.NoError(t, err, "serialization should not fail")
			assert.NotEmpty(t, data, "serialized data should not be empty")

			// Deserialize
			var decoded P2PMessage
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err, "deserialization should not fail")

			// Verify fields
			assert.Equal(t, tt.message.Type, decoded.Type, "message type should match")
			assert.Equal(t, tt.message.Timestamp, decoded.Timestamp, "timestamp should match")
		})
	}
}

func TestP2PMessageDeserializationInvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json}`)
	var msg P2PMessage
	err := json.Unmarshal(invalidJSON, &msg)
	assert.Error(t, err, "should return error for invalid JSON")
}

func TestP2PMessageDeserializationEmptyJSON(t *testing.T) {
	emptyJSON := []byte(`{}`)
	var msg P2PMessage
	err := json.Unmarshal(emptyJSON, &msg)
	assert.NoError(t, err, "should not return error for empty JSON")
	assert.Equal(t, P2PMessageType(""), msg.Type, "type should be empty string")
	assert.Equal(t, int64(0), msg.Timestamp, "timestamp should be 0")
	assert.Nil(t, msg.Data, "data should be nil")
}

func TestP2PMessageDeserializationPartialData(t *testing.T) {
	partialJSON := []byte(`{"type": "play"}`)
	var msg P2PMessage
	err := json.Unmarshal(partialJSON, &msg)
	assert.NoError(t, err, "should not return error for partial JSON")
	assert.Equal(t, P2PMessageType("play"), msg.Type, "type should be 'play'")
	assert.Equal(t, int64(0), msg.Timestamp, "timestamp should be 0")
}

func TestP2PServiceGetLocalPeerID(t *testing.T) {
	service := NewP2PService()
	peerID := service.GetLocalPeerID()
	assert.NotEmpty(t, peerID, "localPeerID should not be empty")
	assert.Equal(t, 16, len(peerID), "localPeerID should be 16 characters")
}

func TestP2PServiceGetRoomID(t *testing.T) {
	service := NewP2PService()

	// Initially roomID should be empty
	assert.Empty(t, service.GetRoomID(), "roomID should be empty before creating/joining room")
}

func TestP2PServiceIsHost(t *testing.T) {
	service := NewP2PService()

	// Initially should not be host
	assert.False(t, service.IsHost(), "should not be host initially")
}

func TestP2PServiceGetPeers(t *testing.T) {
	service := NewP2PService()

	// Initially should have no peers
	peers := service.GetPeers()
	assert.Empty(t, peers, "should have no peers initially")
}

func TestRoomIDLength(t *testing.T) {
	// Test multiple times to ensure consistent length
	for i := 0; i < 50; i++ {
		roomID := generateRoomID()
		assert.Equal(t, 16, len(roomID), "roomID should always be 16 characters, got: %s (len=%d)", roomID, len(roomID))
	}
}

func TestPeerIDLength(t *testing.T) {
	// Test multiple times to ensure consistent length
	for i := 0; i < 50; i++ {
		peerID := generatePeerID()
		assert.Equal(t, 16, len(peerID), "peerID should always be 16 characters, got: %s (len=%d)", peerID, len(peerID))
	}
}

func TestRoomIDFormat(t *testing.T) {
	roomID := generateRoomID()
	assert.True(t, isHexString(roomID), "roomID should be a valid hex string")
}

func TestPeerIDFormat(t *testing.T) {
	peerID := generatePeerID()
	assert.True(t, isHexString(peerID), "peerID should be a valid hex string")
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func TestP2PMessageTypes(t *testing.T) {
	// Verify message type constants
	assert.Equal(t, P2PMessageType("play"), MsgPlay)
	assert.Equal(t, P2PMessageType("pause"), MsgPause)
	assert.Equal(t, P2PMessageType("seek"), MsgSeek)
	assert.Equal(t, P2PMessageType("heartbeat"), MsgHeartbeat)
	assert.Equal(t, P2PMessageType("state"), MsgState)
	assert.Equal(t, P2PMessageType("chat"), MsgChat)
	assert.Equal(t, P2PMessageType("torrent_info"), MsgTorrentInfo)
}

func TestP2PMessageWithComplexData(t *testing.T) {
	// Test with nested data structure
	data := map[string]interface{}{
		"nested": map[string]interface{}{
			"key":    "value",
			"number": 42,
		},
		"array": []interface{}{1, 2, 3},
	}

	msg := P2PMessage{
		Type:      MsgState,
		Timestamp: 1234567890,
		Data:      data,
	}

	serialized, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded P2PMessage
	err = json.Unmarshal(serialized, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.Type, decoded.Type)
	assert.Equal(t, msg.Timestamp, decoded.Timestamp)

	// Verify data is present (it will be map[string]interface{})
	assert.NotNil(t, decoded.Data)
}

func TestP2PMessageWithSpecialCharacters(t *testing.T) {
	// Test with special characters in data
	data := map[string]interface{}{
		"text": "Hello! @#$%^&*() Привет мир! 🎬",
	}

	msg := P2PMessage{
		Type:      MsgChat,
		Timestamp: 1234567890,
		Data:      data,
	}

	serialized, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded P2PMessage
	err = json.Unmarshal(serialized, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.Type, decoded.Type)
}

func TestP2PServiceCloseWithoutInit(t *testing.T) {
	// Should not panic when closing without initialization
	service := NewP2PService()
	assert.NotPanics(t, func() {
		service.Close()
	})
}

func TestP2PServiceDisconnectWithoutPeers(t *testing.T) {
	// Should not panic when disconnecting without peers
	service := NewP2PService()
	assert.NotPanics(t, func() {
		service.Disconnect()
	})
}

func TestP2PMessageEmptyType(t *testing.T) {
	msg := P2PMessage{
		Type:      "",
		Timestamp: 0,
		Data:      nil,
	}

	serialized, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded P2PMessage
	err = json.Unmarshal(serialized, &decoded)
	require.NoError(t, err)

	assert.Equal(t, P2PMessageType(""), decoded.Type)
}

func TestP2PMessageLargeTimestamp(t *testing.T) {
	// Test with maximum int64 value
	msg := P2PMessage{
		Type:      MsgHeartbeat,
		Timestamp: 9223372036854775807, // max int64
		Data:      nil,
	}

	serialized, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded P2PMessage
	err = json.Unmarshal(serialized, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(9223372036854775807), decoded.Timestamp)
}

func TestP2PMessageNegativeTimestamp(t *testing.T) {
	msg := P2PMessage{
		Type:      MsgHeartbeat,
		Timestamp: -1,
		Data:      nil,
	}

	serialized, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded P2PMessage
	err = json.Unmarshal(serialized, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(-1), decoded.Timestamp)
}

func TestGenerateRoomIDContainsOnlyLowercaseHex(t *testing.T) {
	for i := 0; i < 100; i++ {
		roomID := generateRoomID()
		assert.Equal(t, strings.ToLower(roomID), roomID, "roomID should be lowercase hex")
	}
}

func TestGeneratePeerIDContainsOnlyLowercaseHex(t *testing.T) {
	for i := 0; i < 100; i++ {
		peerID := generatePeerID()
		assert.Equal(t, strings.ToLower(peerID), peerID, "peerID should be lowercase hex")
	}
}
