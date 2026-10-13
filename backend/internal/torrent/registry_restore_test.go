// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bug-0053: restoring a torrent after a restart must reproduce the *same*
// infohash.
//
// The previous implementation rebuilt a .torrent from a summary of the
// metadata — name, size, file list — with an empty "pieces" field and a
// hardcoded piece length. The infohash is the SHA-1 of the bencoded info dict,
// so that reconstruction produced a different torrent entirely: rooms bound to
// the original infohash could never bind again, and the ownership map keyed by
// the original hash stopped describing the restored torrent.
//
// The registry must therefore keep the thing that *defines* the identity — the
// magnet URI or the original .torrent bytes — not a description of it.

const testInfoHash = "0123456789abcdef0123456789abcdef01234567"

// metainfoBackend derives a torrent's identity from the real metadata the way
// a real client does, so the test can tell a faithful restore from a forged
// one. The fixed-ID stub would pass either way.
type metainfoBackend struct {
	addedFiles [][]byte
}

func (b *metainfoBackend) AddMagnet(_ context.Context, magnetURI string) (TorrentHandle, error) {
	info, err := metainfo.ParseMagnetUri(magnetURI)
	if err != nil {
		return nil, err
	}
	return &stubHandle{id: info.InfoHash.HexString(), name: "Restored", total: 1, names: []string{"f"}, lengths: []int64{1}}, nil
}

func (b *metainfoBackend) AddTorrentFile(_ context.Context, r io.Reader) (TorrentHandle, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b.addedFiles = append(b.addedFiles, raw)

	mi, err := metainfo.Load(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, err
	}
	return &stubHandle{
		id:      mi.HashInfoBytes().HexString(),
		name:    info.BestName(),
		total:   info.TotalLength(),
		names:   fileNames(info),
		lengths: fileLengths(info),
	}, nil
}

func (b *metainfoBackend) Drop(string)                                                        {}
func (b *metainfoBackend) Close()                                                             {}
func (b *metainfoBackend) ServeStream(http.ResponseWriter, *http.Request, TorrentHandle, int) {}

func fileNames(info metainfo.Info) []string {
	if !info.IsDir() {
		return []string{info.BestName()}
	}
	out := make([]string, 0, len(info.Files))
	for _, f := range info.Files {
		out = append(out, f.BestPath()...)
	}
	return out
}

func fileLengths(info metainfo.Info) []int64 {
	if !info.IsDir() {
		return []int64{info.TotalLength()}
	}
	out := make([]int64, 0, len(info.Files))
	for _, f := range info.Files {
		out = append(out, f.Length)
	}
	return out
}

// A magnet carries its infohash in the URI, so re-adding it by URI is exact by
// construction — as long as the URI is what gets persisted.
func TestRestartRestoresMagnetWithSameInfohash(t *testing.T) {
	dir := t.TempDir()
	store, err := persistence.NewStore(dir)
	require.NoError(t, err)

	magnet := "magnet:?xt=urn:btih:" + testInfoHash + "&dn=movie"

	first := newStubService(t, &metainfoBackend{}, nil)
	first.SetPersistence(store)
	_, err = first.AddMagnet(context.Background(), "owner-peer", magnet)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	// A fresh service over the same data directory: what the operator sees
	// after a restart.
	second := newStubService(t, &metainfoBackend{}, nil)
	second.SetPersistence(store)
	t.Cleanup(func() { _ = second.Close() })

	list := second.ListTorrents(context.Background(), "owner-peer")
	require.Len(t, list, 1, "the torrent did not survive the restart at all")
	assert.Equal(t, testInfoHash, list[0].ID,
		"restored under a different infohash — rooms bound to the original can never bind again")

	// Ownership must survive too, or bug-0049's ownership gate stops working.
	assert.True(t, second.CanRead(context.Background(), "owner-peer", testInfoHash))
}

// The same must hold for a .torrent upload, where the identity lives in the
// bencoded bytes rather than in a URI.
func TestRestartRestoresTorrentFileWithSameInfohash(t *testing.T) {
	dir := t.TempDir()
	store, err := persistence.NewStore(dir)
	require.NoError(t, err)

	raw := buildSingleFileTorrent(t, "movie.mkv", 4096)

	first := newStubService(t, &metainfoBackend{}, nil)
	first.SetPersistence(store)
	info, err := first.AddTorrent(context.Background(), "owner-peer", bytes.NewReader(raw))
	require.NoError(t, err)
	require.NoError(t, first.Close())

	secondBackend := &metainfoBackend{}
	second := newStubService(t, secondBackend, nil)
	second.SetPersistence(store)
	t.Cleanup(func() { _ = second.Close() })

	list := second.ListTorrents(context.Background(), "owner-peer")
	require.Len(t, list, 1)
	assert.Equal(t, info.ID, list[0].ID, "restored under a different infohash")

	require.Len(t, secondBackend.addedFiles, 1)
	assert.Equal(t, raw, secondBackend.addedFiles[0],
		"restore did not hand the backend the original bytes")
}

// buildSingleFileTorrent produces a real, valid .torrent for a single file,
// so the identity under test is a genuine SHA-1 over a genuine info dict.
func buildSingleFileTorrent(t *testing.T, name string, length int64) []byte {
	t.Helper()

	const pieceLength = 16384
	numPieces := (length + pieceLength - 1) / pieceLength
	if numPieces == 0 {
		numPieces = 1
	}
	pieces := make([]byte, numPieces*20)
	for i := range pieces {
		pieces[i] = byte(i)
	}

	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]interface{}{
		"info": map[string]interface{}{
			"name":         name,
			"piece length": pieceLength,
			"pieces":       string(pieces),
			"length":       length,
		},
	}))
	return buf.Bytes()
}
