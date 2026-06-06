package version

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfo(t *testing.T) {
	info := Info()
	assert.NotNil(t, info)
	assert.Equal(t, Version, info["version"])
	assert.Equal(t, Commit, info["commit"])
	assert.Equal(t, BuildTime, info["build"])

	rt, ok := info["runtime"].(map[string]string)
	assert.True(t, ok)
	assert.Equal(t, runtime.Version(), rt["go"])
	assert.Equal(t, runtime.GOARCH, rt["arch"])
	assert.Equal(t, runtime.GOOS, rt["os"])
}

func TestInfo_DefaultValues(t *testing.T) {
	info := Info()
	assert.Equal(t, "dev", info["version"])
	assert.Equal(t, "unknown", info["commit"])
	assert.Equal(t, "unknown", info["build"])
}
