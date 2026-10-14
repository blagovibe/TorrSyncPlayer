// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE for full license text

package torrent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Removing a torrent has to free the disk it occupied.
//
// The API answers 200 and the torrent leaves the list either way, so "the data
// is still there" is invisible to every check that does not look at the disk —
// which is why the deletion path was written from a plausible reading of the
// storage layout instead of the layout itself, and why nothing noticed that it
// removed <dataDir>/<infohash> while anacrolix writes <dataDir>/<info name>.

func TestRemovingADiskBackedTorrentFreesItsData(t *testing.T) {
	requireFileBackedPieces(t)

	ctx := context.Background()
	payload := testPayload(swarmPieceLength * 4)
	info := buildSwarmTorrent(t, payload, startLoopbackTracker(t))

	base := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("data dir: %v", err)
	}

	// anacrolix lays a torrent down as <dataDir>/<Info.BestName()>.
	onDisk := filepath.Join(base, info.Name)
	if err := os.WriteFile(onDisk, payload, 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	svc, err := NewServiceWithOptions(nil, ServiceOptions{
		NoDHT: true, DisableUTP: true, Storage: StorageDisk, DataDir: base,
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	defer svc.Close()

	got, err := svc.AddTorrent(ctx, "owner", bytes.NewReader(info.Raw))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	waitForPieces(t, svc, nil, got.ID, 4, 30*time.Second)

	before, err := os.Stat(onDisk)
	if err != nil || before.Size() != int64(len(payload)) {
		t.Fatalf("the torrent is not on disk in the first place: %v", err)
	}

	if err := svc.RemoveTorrent(ctx, got.ID, "owner"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := os.Stat(onDisk); !os.IsNotExist(err) {
		t.Errorf("the torrent was removed and %d bytes of its data are still on disk", int64(len(payload)))
	}
}
