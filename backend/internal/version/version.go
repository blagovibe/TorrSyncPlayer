// SPDX-License-Identifier: MIT

// Package version provides server version information.
// Variables are set at build time via -ldflags.
package version

import "runtime"

// Versioning (set at build time via -ldflags)
var (
	// Version application version
	Version = "dev"

	// Commit git commit hash
	Commit = "unknown"

	// BuildTime build time
	BuildTime = "unknown"
)

// Info returns version information
func Info() map[string]interface{} {
	return map[string]interface{}{
		"version": Version,
		"commit":  Commit,
		"build":   BuildTime,
		"runtime": map[string]string{
			"go":   runtime.Version(),
			"arch": runtime.GOARCH,
			"os":   runtime.GOOS,
		},
	}
}
