// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
)

// The software painter rasterises the whole window into an image, so the
// design can be checked without a graphics driver — and on a machine where
// nobody can open the window. Set TSP_SHOT to a path to write the frames
// ("-dark.png" / "-light.png" are appended); the default is a temporary
// file. TSP_FIXTURE=1 feeds the window a two-film shelf and a playing film,
// which is what the real screen looks like mid-session.
func TestTheWindowRendersItsMoods(t *testing.T) {
	fixture := os.Getenv("TSP_FIXTURE") == "1"
	debugHeight := float32(800)
	if h := os.Getenv("TSP_H"); h != "" {
		fmt.Sscanf(h, "%f", &debugHeight)
	}

	for _, mood := range []struct {
		name string
		dark bool
	}{{"dark", true}, {"light", false}} {
		t.Run(mood.name, func(t *testing.T) {
			a := test.NewApp()
			InstallTorrTheme(a, mood.dark)

			clientURL := "http://127.0.0.1:1"
			var srv *httptest.Server
			if fixture {
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case r.URL.Path == "/api/v1/torrents" && r.Method == http.MethodGet:
						_, _ = io.WriteString(w, `{"torrents":[`+
							`{"id":"t1","name":"Jimmy Neutron: Boy Genius (2001).mkv","size":2384185792,"progress":0.42,"status":"downloading","peers":3},`+
							`{"id":"t2","name":"Moscow Zero (S01E01).mkv","size":734003200,"progress":1,"status":"seeding","peers":8}`+
							`]}`)
					case strings.HasSuffix(r.URL.Path, "/files"):
						_, _ = io.WriteString(w, `{"files":[`+
							`{"index":0,"name":"Jimmy Neutron: Boy Genius (2001).mkv","size":2384185792},`+
							`{"index":1,"name":"Jimmy.Neutron.2001.RU.srt","size":98304},`+
							`{"index":2,"name":"sample.mkv","size":10485760}`+
							`]}`)
					case strings.HasSuffix(r.URL.Path, "/magnet"):
						_, _ = io.WriteString(w, `{"magnetUri":"magnet:?xt=urn:btih:abc&tr=http://tracker.example/x"}`)
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				defer srv.Close()
				clientURL = srv.URL
			}

			client := api.New(clientURL, "test-token")
			w, content := newWindow(client, "peer-test", nil, WindowOptions{})
			if w == nil || content == nil {
				t.Fatal("newWindow returned no content")
			}

			if fixture {
				w.refresh()
				w.refresh()
				w.mu.Lock()
				w.current = &Torrent{ID: "t1", Name: "Jimmy Neutron: Boy Genius (2001).mkv", Size: 2384185792}
				w.ctlHeroName = w.current.Name
				w.files = []fileEntry{
					{Index: 0, Name: "Jimmy Neutron: Boy Genius (2001).mkv", Size: 2384185792},
					{Index: 1, Name: "Jimmy.Neutron.2001.RU.srt", Size: 98304},
					{Index: 2, Name: "sample.mkv", Size: 10485760},
				}
				w.rates["t1"] = rateSample{bytes: 400, rate: 2_750_000}
				w.mu.Unlock()
				w.heroCaption.Show()
				w.heroAdd.Hide()
				w.heroLbl.SetText(w.current.Name)
				w.heroSubLbl.Text = "2.2 GB · 2:05:42"
				w.heroSubLbl.Refresh()
				w.setControlsEnabled(true)
				w.seekSlider.Max = 7542
				setSliderValue(w.seekSlider, w.onSeekMoved, 2320)
				w.elapsedLbl.Text = "38:40"
				w.elapsedLbl.Refresh()
				w.durationLbl.Text = "2:05:42"
				w.durationLbl.Refresh()
				w.volSlider.SetValue(70)
				w.createRoom.Enable()
				w.rebuildShelf()
				w.rebuildFileChips()
			}

			canvas := software.NewCanvas()
			canvas.SetContent(content)
			canvas.Resize(fyne.NewSize(1280, debugHeight))

			img := canvas.Capture()
			if img == nil || img.Bounds().Dx() <= 0 {
				t.Fatal("the capture came back empty")
			}

			path := os.Getenv("TSP_SHOT")
			if path == "" {
				path = filepath.Join(t.TempDir(), "window-"+mood.name+".png")
			} else {
				ext := filepath.Ext(path)
				path = strings.TrimSuffix(path, ext) + "-" + mood.name + ext
			}
			file, err := os.Create(path)
			if err != nil {
				t.Fatalf("create %s: %v", path, err)
			}
			defer file.Close()
			if err := png.Encode(file, img); err != nil {
				t.Fatalf("encode %s: %v", path, err)
			}
			t.Logf("frame written to %s", path)
		})
	}
}
