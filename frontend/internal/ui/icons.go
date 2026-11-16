// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The application's own icon set.
//
// Fyne's built-in icons are a good plain default, but the window promised
// its own visual language (idea-0187), and a flat row of the stock set is one
// of the reasons it read as a debug panel. These are drawn on one 24-pixel
// grid with the same rules: a single silhouette per glyph, fills only (the
// theme colouriser repaints every fill, so a glyph that split itself across
// two colours would arrive as one blob), no strokes, and an empty backplate
// path kept out of the paint (`fill="none"`) so the glyph sits on its real
// background instead of a coloured square.
//
// Simple glyphs are drawn with primitives (rounded rectangles, arc circles,
// hand-cornered paths); compound glyphs reuse the geometry of the Material
// set the rest of the window was born with — familiar beats novel for a
// trash can, and novel buys no clarity on a settings gear.

const torrPlaySVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M8 8.23 L8 15.77 Q8 18.5 10.3 17.03 L16.2 13.27 Q18.2 12 16.2 10.73 L10.3 6.97 Q8 5.5 8 8.23 Z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrPauseSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<rect x="7.1" y="5.2" width="3.6" height="13.6" rx="1.5"/>` +
	`<rect x="13.3" y="5.2" width="3.6" height="13.6" rx="1.5"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrFastRewindSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M11 18V6l-8.5 6L11 18zm.5-6l8.5 6V6l-8.5 6z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrFastForwardSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M4 18l8.5-6L4 6v12zm9-12v12l8.5-6L13 6z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

// Four rounded corner brackets: the frame opens to the edges of the screen.
const torrFullscreenSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M4 9.5 L4 5.5 Q4 4 5.5 4 L9.5 4 L9.5 6.2 L6.2 6.2 L6.2 9.5 Z"/>` +
	`<path d="M20 9.5 L20 5.5 Q20 4 18.5 4 L14.5 4 L14.5 6.2 L17.8 6.2 L17.8 9.5 Z"/>` +
	`<path d="M4 14.5 L4 18.5 Q4 20 5.5 20 L9.5 20 L9.5 17.8 L6.2 17.8 L6.2 14.5 Z"/>` +
	`<path d="M20 14.5 L20 18.5 Q20 20 18.5 20 L14.5 20 L14.5 17.8 L17.8 17.8 L17.8 14.5 Z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

// Corners pointing back into the middle: the screen closes again.
const torrFullscreenExitSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrAddSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<rect x="11" y="4.6" width="2" height="14.8" rx="1"/>` +
	`<rect x="4.6" y="11" width="14.8" height="2" rx="1"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrSettingsSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrRefreshSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M17.65 6.35C16.2 4.9 14.21 4 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrFolderSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

// A crowd: three seated figures, the middle one in front and larger —
// the silhouette of people sharing exactly one screen.
const torrRoomSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M3.2 10.5a2.1 2.1 0 1 0 4.2 0 2.1 2.1 0 0 0-4.2 0Z"/>` +
	`<path d="M16.6 10.5a2.1 2.1 0 1 0 4.2 0 2.1 2.1 0 0 0-4.2 0Z"/>` +
	`<rect x="1.4" y="14.9" width="7.8" height="3.8" rx="1.9"/>` +
	`<rect x="14.8" y="14.9" width="7.8" height="3.8" rx="1.9"/>` +
	`<path d="M9.05 8.1a2.95 2.95 0 1 0 5.9 0 2.95 2.95 0 0 0-5.9 0Z"/>` +
	`<rect x="6.35" y="12.9" width="11.3" height="6" rx="3"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

// A doorframe with an arrow pointing out of it: leaving the room. The join
// glyph below is the same door mirrored, arrow entering instead — the pair
// reads together the way login and logout do.
const torrLeaveRoomSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M4 5.2A1.2 1.2 0 0 1 5.2 4h7.2v2.3H6.3v11.4h6.1V20H5.2A1.2 1.2 0 0 1 4 18.8z"/>` +
	`<rect x="10.6" y="10.95" width="6.6" height="2.1" rx="1.05"/>` +
	`<path d="M15.3 8.15 L20.1 11.7 L15.3 15.25 Z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrJoinRoomSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M20 5.2A1.2 1.2 0 0 0 18.8 4h-7.1v2.3h6.1v11.4h-6.1V20h7.1A1.2 1.2 0 0 0 20 18.8z"/>` +
	`<rect x="8.1" y="10.95" width="6.6" height="2.1" rx="1.05"/>` +
	`<path d="M8.7 8.15 L13.5 11.7 L8.7 15.25 Z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrInfoSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrDeleteSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrCopySVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrPasteSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M19 3h-4.18C14.4 1.84 13.3 1 12 1c-1.3 0-2.4.84-2.82 2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-7 0c.55 0 1 .45 1 1s-.45 1-1 1-1-.45-1-1 .45-1 1-1zm7 16H5V5h2v3h10V5h2v14z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrCheckSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

const torrVolumeUpSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

// A horseshoe magnet: the shape the one text field in the window exists for.
const torrMagnetSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path d="M6.5 3.5v7a5.5 5.5 0 0 0 11 0v-7h-3.6v7a1.9 1.9 0 0 1-3.8 0v-7H6.5z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

// A film strip with punched sprockets — the emblem of the house. Evens on
// the strip are holes, and `evenodd` keeps them holes for every theme.
const torrFilmSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<path fill-rule="evenodd" d="M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1z` +
	` M6.8 8.2h2.4v1.9H6.8z M10.8 8.2h2.4v1.9h-2.4z M14.8 8.2H17.2v1.9h-2.4z` +
	` M6.8 14h2.4v1.9H6.8z M10.8 14h2.4v1.9h-2.4z M14.8 14H17.2v1.9h-2.4z"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

// Three quiet dots: the door to an item's own actions, kept off the title.
const torrDotsSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="24" viewBox="0 0 24 24" width="24">` +
	`<circle cx="4.6" cy="12" r="1.9"/>` +
	`<circle cx="12" cy="12" r="1.9"/>` +
	`<circle cx="19.4" cy="12" r="1.9"/>` +
	`<path d="M0 0h24v24H0z" fill="none"/></svg>`

// torrIcons is every icon in the set, name → source, so the tests can chew
// through the whole collection without keeping a second list by hand.
var torrIcons = map[string]string{
	"play":            torrPlaySVG,
	"pause":           torrPauseSVG,
	"fast-rewind":     torrFastRewindSVG,
	"fast-forward":    torrFastForwardSVG,
	"fullscreen":      torrFullscreenSVG,
	"fullscreen-exit": torrFullscreenExitSVG,
	"add":             torrAddSVG,
	"settings":        torrSettingsSVG,
	"refresh":         torrRefreshSVG,
	"folder":          torrFolderSVG,
	"room":            torrRoomSVG,
	"leave-room":      torrLeaveRoomSVG,
	"join-room":       torrJoinRoomSVG,
	"info":            torrInfoSVG,
	"delete":          torrDeleteSVG,
	"copy":            torrCopySVG,
	"paste":           torrPasteSVG,
	"check":           torrCheckSVG,
	"volume-up":       torrVolumeUpSVG,
	"magnet":          torrMagnetSVG,
	"film":            torrFilmSVG,
	"dots":            torrDotsSVG,
}

// themed wraps a raw SVG source in the theme-colourised resource the window
// uses everywhere: Fyne repaints every fill with the current foreground
// colour at draw time, so one glyph works on the toolbar, in a button and
// on a selection without a single hard-coded pixel of colour.
func themed(name, src string) fyne.Resource {
	return theme.NewThemedResource(&fyne.StaticResource{
		StaticName:    "torr-" + name + ".svg",
		StaticContent: []byte(src),
	})
}

var (
	torrPlayIcon           = themed("play", torrPlaySVG)
	torrPauseIcon          = themed("pause", torrPauseSVG)
	torrFastRewindIcon     = themed("fast-rewind", torrFastRewindSVG)
	torrFastForwardIcon    = themed("fast-forward", torrFastForwardSVG)
	torrFullscreenIcon     = themed("fullscreen", torrFullscreenSVG)
	torrFullscreenExitIcon = themed("fullscreen-exit", torrFullscreenExitSVG)
	torrAddIcon            = themed("add", torrAddSVG)
	torrSettingsIcon       = themed("settings", torrSettingsSVG)
	torrRefreshIcon        = themed("refresh", torrRefreshSVG)
	torrFolderIcon         = themed("folder", torrFolderSVG)
	torrRoomIcon           = themed("room", torrRoomSVG)
	torrLeaveRoomIcon      = themed("leave-room", torrLeaveRoomSVG)
	torrJoinRoomIcon       = themed("join-room", torrJoinRoomSVG)
	torrInfoIcon           = themed("info", torrInfoSVG)
	torrDeleteIcon         = themed("delete", torrDeleteSVG)
	torrCopyIcon           = themed("copy", torrCopySVG)
	torrPasteIcon          = themed("paste", torrPasteSVG)
	torrCheckIcon          = themed("check", torrCheckSVG)
	torrVolumeUpIcon       = themed("volume-up", torrVolumeUpSVG)
	torrMagnetIcon         = themed("magnet", torrMagnetSVG)
	torrFilmIcon           = themed("film", torrFilmSVG)
	torrDotsIcon           = themed("dots", torrDotsSVG)
)
