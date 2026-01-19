package fec

import (
	"bytes"
	"testing"
)

// TestDecodeWithMissingFirstShard tests FEC decoding when the first shard is missing
// This simulates the scenario where shards arrive out of order in large UDP packets
func TestDecodeWithMissingFirstShard(t *testing.T) {
	// Create FEC with 3 data shards and 2 parity shards
	dataShards := 3
	parityShards := 2
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	// Original data
	originalData := []byte("This is a test message for FEC encoding and decoding with missing first shard")

	// Encode the data
	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	// Simulate missing first shard (shards[0])
	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}
	shardPresent[0] = false // First shard is missing
	
	// Save the first shard for comparison, but set it to nil in the array
	firstShard := shards[0]
	shards[0] = nil

	// Attempt to decode with missing first shard
	decoded, err := fec.Decode(shards, shardPresent)
	if err != nil {
		t.Fatalf("Failed to decode with missing first shard: %v", err)
	}

	// Trim decoded data to original size
	if len(decoded) > len(originalData) {
		decoded = decoded[:len(originalData)]
	}

	// Verify the decoded data matches original
	if !bytes.Equal(decoded, originalData) {
		t.Errorf("Decoded data doesn't match original.\nExpected: %s\nGot: %s", originalData, decoded)
	}

	// Also verify that the reconstructed first shard matches the original
	if !bytes.Equal(shards[0], firstShard) {
		t.Error("Reconstructed first shard doesn't match original")
	}
}

// TestDecodeWithMissingMiddleShard tests FEC decoding when a middle shard is missing
func TestDecodeWithMissingMiddleShard(t *testing.T) {
	dataShards := 4
	parityShards := 2
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	originalData := []byte("Testing FEC with missing middle shard for large UDP packets")

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	// Simulate missing middle shard (shards[2])
	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}
	shardPresent[2] = false

	middleShard := shards[2]
	shards[2] = nil

	decoded, err := fec.Decode(shards, shardPresent)
	if err != nil {
		t.Fatalf("Failed to decode with missing middle shard: %v", err)
	}

	if len(decoded) > len(originalData) {
		decoded = decoded[:len(originalData)]
	}

	if !bytes.Equal(decoded, originalData) {
		t.Errorf("Decoded data doesn't match original")
	}

	if !bytes.Equal(shards[2], middleShard) {
		t.Error("Reconstructed middle shard doesn't match original")
	}
}

// TestDecodeAllShardsPresent tests normal FEC decoding when all shards are present
func TestDecodeAllShardsPresent(t *testing.T) {
	dataShards := 5
	parityShards := 2
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	originalData := []byte("Testing FEC with all shards present - normal operation mode")

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	// All shards present
	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}

	decoded, err := fec.Decode(shards, shardPresent)
	if err != nil {
		t.Fatalf("Failed to decode with all shards present: %v", err)
	}

	if len(decoded) > len(originalData) {
		decoded = decoded[:len(originalData)]
	}

	if !bytes.Equal(decoded, originalData) {
		t.Errorf("Decoded data doesn't match original")
	}
}

// TestDecodeInsufficientShards tests that decoding fails when not enough shards are present
func TestDecodeInsufficientShards(t *testing.T) {
	dataShards := 3
	parityShards := 2
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	originalData := []byte("Testing insufficient shards")

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	// Only 2 shards present (need at least 3)
	shardPresent := make([]bool, len(shards))
	shardPresent[0] = true
	shardPresent[1] = true
	// All others are false

	_, err = fec.Decode(shards, shardPresent)
	if err == nil {
		t.Error("Expected error when not enough shards present, but got nil")
	}
}

// TestDecodeLargeData tests FEC with larger data similar to large UDP packets
func TestDecodeLargeData(t *testing.T) {
	dataShards := 10
	parityShards := 3
	shardSize := 1024
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	// Create large data (simulating a large UDP packet)
	originalData := make([]byte, 8192)
	for i := range originalData {
		originalData[i] = byte(i % 256)
	}

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	// Simulate missing first shard only
	// Note: This simple XOR-based FEC implementation has limited recovery capabilities.
	// It creates parity shards that are XOR of all data shards, allowing recovery of
	// only one missing data shard at a time (not one per parity shard).
	// For production use with better recovery, consider proper Reed-Solomon encoding.
	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}
	shardPresent[0] = false

	shards[0] = nil

	decoded, err := fec.Decode(shards, shardPresent)
	if err != nil {
		t.Fatalf("Failed to decode large data: %v", err)
	}

	if len(decoded) > len(originalData) {
		decoded = decoded[:len(originalData)]
	}

	if !bytes.Equal(decoded, originalData) {
		t.Errorf("Decoded large data doesn't match original")
	}
}
