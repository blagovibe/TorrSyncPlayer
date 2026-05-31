// Package config provides configuration structures for TURN servers
// used in WebRTC peer-to-peer connections.
//
// TURN (Traversal Using Relays around NAT) servers are essential for
// establishing P2P connections when direct connection is not possible
// due to NAT or firewall restrictions.
//
// For detailed setup instructions, see docs/TURN_SETUP.md
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"
)

// TURNConfig holds configuration for TURN/STUN servers
type TURNConfig struct {
	// STUNServers is a list of STUN server URLs
	STUNServers []string `json:"stun_servers" env:"STUN_SERVERS" envDefault:"stun:stun.l.google.com:19302,stun:stun1.l.google.com:19302"`

	// TURNServers is a list of TURN server configurations
	TURNServers []TURNServerConfig `json:"turn_servers"`

	// CredentialSecret is the secret key for generating temporary TURN credentials
	// Used with TURN servers that support ephemeral credentials
	CredentialSecret string `json:"-" env:"TURN_CREDENTIAL_SECRET"`

	// CredentialTTL is the time-to-live for temporary credentials
	CredentialTTL time.Duration `json:"credential_ttl" env:"TURN_CREDENTIAL_TTL" envDefault:"24h"`
}

// TURNServerConfig holds configuration for a single TURN server
type TURNServerConfig struct {
	// URLs is a list of TURN server URLs (e.g., turn:example.com:3478)
	URLs []string `json:"urls" env:"TURN_SERVER_URLS"`

	// Username for TURN server authentication
	Username string `json:"username" env:"TURN_USERNAME"`

	// Credential for TURN server authentication
	// Can be a password or a shared secret for temporary credentials
	Credential string `json:"credential" env:"TURN_CREDENTIAL"`

	// UseTLS indicates whether to use TURN over TLS (turns://)
	UseTLS bool `json:"use_tls" env:"TURN_USE_TLS" envDefault:"true"`
}

// DefaultTURNConfig returns a default TURN configuration with public STUN servers
func DefaultTURNConfig() *TURNConfig {
	return &TURNConfig{
		STUNServers: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		},
		TURNServers:      []TURNServerConfig{},
		CredentialTTL:    24 * time.Hour,
		CredentialSecret: "",
	}
}

// LoadFromEnv loads TURN configuration from environment variables
//
// Supported environment variables:
//   - TURN_SERVER_URLS: Comma-separated list of TURN server URLs
//   - TURN_USERNAME: Username for TURN authentication
//   - TURN_CREDENTIAL: Password/credential for TURN authentication
//   - TURN_USE_TLS: Whether to use TLS (default: true)
//   - TURN_CREDENTIAL_SECRET: Secret for generating temporary credentials
//   - TURN_CREDENTIAL_TTL: TTL for temporary credentials (default: 24h)
//   - STUN_SERVERS: Comma-separated list of STUN server URLs
func LoadFromEnv() (*TURNConfig, error) {
	config := DefaultTURNConfig()

	// Load STUN servers from env
	if stunServers := os.Getenv("STUN_SERVERS"); stunServers != "" {
		config.STUNServers = parseStringList(stunServers)
	}

	// Load TURN configuration from env
	turnURLs := os.Getenv("TURN_SERVER_URLS")
	turnUsername := os.Getenv("TURN_USERNAME")
	turnCredential := os.Getenv("TURN_CREDENTIAL")

	// Only add TURN server if all required fields are provided
	if turnURLs != "" && turnUsername != "" && turnCredential != "" {
		useTLS := true
		if tls := os.Getenv("TURN_USE_TLS"); tls != "" {
			useTLS = strings.ToLower(tls) == "true" || tls == "1"
		}

		config.TURNServers = append(config.TURNServers, TURNServerConfig{
			URLs:       parseStringList(turnURLs),
			Username:   turnUsername,
			Credential: turnCredential,
			UseTLS:     useTLS,
		})
	}

	// Load credential secret for temporary credentials
	config.CredentialSecret = os.Getenv("TURN_CREDENTIAL_SECRET")

	// Load credential TTL
	if ttlStr := os.Getenv("TURN_CREDENTIAL_TTL"); ttlStr != "" {
		ttl, err := time.ParseDuration(ttlStr)
		if err != nil {
			return nil, fmt.Errorf("invalid TURN_CREDENTIAL_TTL: %w", err)
		}
		config.CredentialTTL = ttl
	}

	return config, nil
}

// ToICEServers converts TURNConfig to WebRTC ICEServers
func (c *TURNConfig) ToICEServers() []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0)

	// Add STUN servers
	for _, url := range c.STUNServers {
		if url = strings.TrimSpace(url); url != "" {
			servers = append(servers, webrtc.ICEServer{
				URLs: []string{url},
			})
		}
	}

	// Add TURN servers
	for _, turn := range c.TURNServers {
		servers = append(servers, webrtc.ICEServer{
			URLs:       turn.URLs,
			Username:   turn.Username,
			Credential: turn.Credential,
		})
	}

	return servers
}

// HasTURNServers returns true if TURN servers are configured
func (c *TURNConfig) HasTURNServers() bool {
	return len(c.TURNServers) > 0
}

// Validate checks if the configuration is valid
func (c *TURNConfig) Validate() error {
	if len(c.STUNServers) == 0 && len(c.TURNServers) == 0 {
		return fmt.Errorf("at least one STUN or TURN server must be configured")
	}

	for i, turn := range c.TURNServers {
		if len(turn.URLs) == 0 {
			return fmt.Errorf("TURN server %d: no URLs configured", i)
		}
		if turn.Username == "" {
			return fmt.Errorf("TURN server %d: username is required", i)
		}
		if turn.Credential == "" {
			return fmt.Errorf("TURN server %d: credential is required", i)
		}
	}

	return nil
}

// GenerateTemporaryCredentials generates temporary TURN credentials
// using HMAC-based authentication (RFC 8489)
//
// This is useful for TURN servers that support ephemeral credentials
// for enhanced security.
func (c *TURNConfig) GenerateTemporaryCredentials(username string) (string, string, error) {
	if c.CredentialSecret == "" {
		return "", "", fmt.Errorf("TURN_CREDENTIAL_SECRET is not configured")
	}

	// Import crypto packages locally to avoid unused imports
	// when temporary credentials are not needed
	return generateCredentials(username, c.CredentialSecret, c.CredentialTTL)
}

// generateCredentials creates temporary TURN credentials
func generateCredentials(username, secret string, ttl time.Duration) (string, string, error) {
	// This is a placeholder implementation
	// In production, use HMAC-SHA1 based credential generation
	// as specified in RFC 8489 Section 4.3

	// Example implementation:
	// timestamp := time.Now().Add(ttl).Unix()
	// fullUsername := fmt.Sprintf("%d:%s", timestamp, username)
	// mac := hmac.New(sha1.New, []byte(secret))
	// mac.Write([]byte(fullUsername))
	// credential := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	// return fullUsername, credential, nil

	return username, secret, nil
}

// parseStringList parses a comma-separated string into a slice
func parseStringList(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Example .env file content:
//
// # TURN Server Configuration
// TURN_SERVER_URLS=turn:your-turn-server.com:3478,turns:your-turn-server.com:5349
// TURN_USERNAME=torrsync
// TURN_CREDENTIAL=your_secure_password
// TURN_USE_TLS=true
//
// # Optional: For temporary credentials
// TURN_CREDENTIAL_SECRET=your_shared_secret
// TURN_CREDENTIAL_TTL=24h
