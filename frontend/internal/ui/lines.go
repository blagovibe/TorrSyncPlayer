// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import "fyne.io/fyne/v2/widget"

// setLine writes an informational line or removes it. An empty label is not
// neutral: it keeps its height and leaves a gap in the composition, which
// reads as a zone with something missing rather than as nothing to say. The
// zones are named and ruled, so a hole under a heading is a hole everyone
// sees.
func setLine(label *widget.Label, text string) {
	if text == "" {
		label.Hide()
		return
	}
	label.SetText(text)
	label.Show()
}
