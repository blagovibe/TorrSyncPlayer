// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"os"
	"strings"
	"testing"
)

// The coordinator hands each participant its role, invite and rendezvous as
// command-line arguments. It cannot use environment variables: the first run
// of this scenario on a real runner produced two children that started
// happily and reported that they were not participants, because the role had
// not reached them by the time they looked.
//
// It cannot simply declare flags either: Go's testing package parses the
// command line before any test runs and refuses anything it does not know,
// with a usage message and no test executed. So the arguments are taken out
// here, before that happens, and handed back to the participants through the
// same process that reads them.
var natArgs = map[string]string{}

// natFlagPrefix marks the arguments that belong to this scenario rather than to
// the test binary.
const natFlagPrefix = "-nat-"

// TestMain removes the scenario's own arguments from the command line and runs
// the tests without them. Everything else is left exactly as it was.
func TestMain(m *testing.M) {
	rest := make([]string, 0, len(os.Args))
	args := os.Args
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, natFlagPrefix) {
			rest = append(rest, arg)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, natFlagPrefix), "=")
		if hasValue {
			natArgs[natFlagPrefix+name] = value
			continue
		}
		// `-nat-role host`: the value is the next argument, and a flag left
		// without one is worth saying out loud rather than reading as empty.
		if i+1 < len(args) {
			natArgs[natFlagPrefix+name] = args[i+1]
			i++
			continue
		}
		natArgs[natFlagPrefix+name] = ""
	}
	os.Args = rest
	os.Exit(m.Run())
}

// natFlag reads one of the coordinator's arguments.
func natFlag(name string) string { return natArgs[name] }
