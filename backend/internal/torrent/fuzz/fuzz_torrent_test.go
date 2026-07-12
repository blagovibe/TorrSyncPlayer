/**
 * @file fuzz_torrent_test.go
 * @brief Go fuzzing tests for torrent parsing and validation
 * 
 * Run with: go test -fuzz=Fuzz -fuzztime=30s ./internal/torrent/...
 */

package torrent

import (
	"bytes"
	"strings"
	"testing"
)

// FuzzParseMagnetURI tests magnet URI parsing with random inputs
func FuzzParseMagnetURI(f *testing.F) {
	// Seed corpus with valid magnet URIs
	validMagnets := []string{
		"magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890abcdef12",
		"magnet:?xt=urn:btih:ABCDEF1234567890ABCDEF1234567890ABCDEF12&dn=Test%20File",
		"magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&tr=udp%3A%2F%2Ftracker.example.com%3A80",
		"magnet:?xt=urn:btih:fedcba0987654321fedcba0987654321fedcba09&xs=http%3A%2F%2Fexample.com%2Ffile.torrent",
	}
	
	for _, m := range validMagnets {
		f.Add(m)
	}
	
	f.Fuzz(func(t *testing.T, data string) {
		// Test that parsing doesn't panic
		result := ParseMagnetURI(data)
		
		// If it looks like a magnet URI, it should parse something
		if strings.HasPrefix(data, "magnet:?") {
			// Basic validation - should not crash
			_ = result
		}
	})
}

// FuzzParseTorrentFile tests .torrent file parsing with random bencoded data
func FuzzParseTorrentFile(f *testing.F) {
	// Seed with valid minimal torrent structures
	validTorrents := [][]byte{
		// Minimal valid torrent: d4:infod6:lengthi100e4:name8:test.txtee
		[]byte("d4:infod6:lengthi100e4:name8:test.txtee"),
		// Torrent with multiple files
		[]byte("d4:infod5:filesld6:lengthi100e4:pathl8:folder8:file1.txteed6:lengthi200e4:pathl8:folder8:file2.txteeee4:name7:myfiles6:pieces20:aaaaaaaaaaaaaaaaaaaaee"),
		// Torrent with announce list
		[]byte("d4:infod6:lengthi100e4:name8:test.txted8:announce41:http://tracker.example.com/announceee"),
	}
	
	for _, t := range validTorrents {
		f.Add(t)
	}
	
	f.Fuzz(func(t *testing.T, data []byte) {
		// Test that parsing doesn't panic
		_, err := ParseTorrentFile(bytes.NewReader(data))
		
		// Errors are expected for invalid data, but no panics
		_ = err
	})
}

// FuzzInfoHash tests info hash calculation with random inputs
func FuzzInfoHash(f *testing.F) {
	validInfos := [][]byte{
		[]byte("d6:lengthi100e4:name8:test.txtee"),
		[]byte("d5:filesld6:lengthi100e4:pathl8:folder8:file1.txteeeee"),
	}
	
	for _, info := range validInfos {
		f.Add(info)
	}
	
	f.Fuzz(func(t *testing.T, data []byte) {
		// Test info hash calculation doesn't panic
		hash := CalculateInfoHash(data)
		
		// If valid, should be 20 bytes (SHA1)
		if len(hash) > 0 && len(hash) != 20 {
			t.Errorf("Invalid info hash length: %d", len(hash))
		}
	})
}

// FuzzValidateMagnetURI tests magnet URI validation
func FuzzValidateMagnetURI(f *testing.F) {
	f.Add("magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890abcdef12")
	f.Add("magnet:?xt=urn:btih:invalid")
	f.Add("not-a-magnet")
	f.Add("")
	
	f.Fuzz(func(t *testing.T, data string) {
		valid := ValidateMagnetURI(data)
		_ = valid // Just ensure no panic
	})
}

// FuzzTorrentNameSanitization tests filename sanitization
func FuzzTorrentNameSanitization(f *testing.F) {
	f.Add("normal_file.txt")
	f.Add("file with spaces.mp4")
	f.Add("файл_на_русском.mkv")
	f.Add("文件名_中文.avi")
	f.Add("file<script>alert(1)</script>.mp4")
	f.Add("file\0null.txt")
	f.Add(strings.Repeat("a", 1000))
	
	f.Fuzz(func(t *testing.T, data string) {
		result := SanitizeFilename(data)
		
		// Result should not contain null bytes
		if strings.Contains(result, "\x00") {
			t.Error("Sanitized filename contains null byte")
		}
		
		// Result should not be longer than reasonable limit
		if len(result) > 255 {
			t.Errorf("Sanitized filename too long: %d", len(result))
		}
	})
}

// FuzzPeerIDGeneration tests peer ID generation
func FuzzPeerIDGeneration(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		// Generate peer ID from random data
		id := GeneratePeerID(data)
		
		// Should be 20 bytes
		if len(id) != 20 {
			t.Errorf("Peer ID length: %d, expected 20", len(id))
		}
		
		// Should be valid ASCII
		for _, b := range id {
			if b < 32 || b > 126 {
				t.Errorf("Peer ID contains non-ASCII byte: %d", b)
			}
		}
	})
}