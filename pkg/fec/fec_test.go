package fec

import (
	"bytes"
	"testing"
)

// TestDecodeWithMissingFirstShard tests FEC decoding when the first shard is missing
func TestDecodeWithMissingFirstShard(t *testing.T) {
	dataShards := 3
	parityShards := 2
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	originalData := []byte("This is a test message for FEC encoding and decoding with missing first shard")

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}
	shardPresent[0] = false // First shard is missing

	firstShard := shards[0]
	shards[0] = nil

	decoded, err := fec.Decode(shards, shardPresent)
	if err != nil {
		t.Fatalf("Failed to decode with missing first shard: %v", err)
	}

	if len(decoded) > len(originalData) {
		decoded = decoded[:len(originalData)]
	}

	if !bytes.Equal(decoded, originalData) {
		t.Errorf("Decoded data doesn't match original.\nExpected: %s\nGot: %s", originalData, decoded)
	}

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

	originalData := make([]byte, 8192)
	for i := range originalData {
		originalData[i] = byte(i % 256)
	}

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

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

// TestDecodeMultipleDataShardsMissing tests recovery of multiple missing data shards
func TestDecodeMultipleDataShardsMissing(t *testing.T) {
	dataShards := 10
	parityShards := 3
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	originalData := make([]byte, 950)
	for i := range originalData {
		originalData[i] = byte(i % 256)
	}

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}

	// Lose shards 0, 3, 7
	shardPresent[0] = false
	shardPresent[3] = false
	shardPresent[7] = false

	shards[0] = nil
	shards[3] = nil
	shards[7] = nil

	decoded, err := fec.Decode(shards, shardPresent)
	if err != nil {
		t.Fatalf("Failed to decode with multiple missing data shards: %v", err)
	}

	if len(decoded) > len(originalData) {
		decoded = decoded[:len(originalData)]
	}

	if !bytes.Equal(decoded, originalData) {
		t.Errorf("Decoded data doesn't match original")
	}
}

// TestDecodeMultipleMixedMissing tests recovery of mixed missing data and parity shards
func TestDecodeMultipleMixedMissing(t *testing.T) {
	dataShards := 10
	parityShards := 5
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	originalData := make([]byte, 950)
	for i := range originalData {
		originalData[i] = byte((i * 17) % 256)
	}

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}

	// Lose 2 data shards and 2 parity shards
	shardPresent[2] = false
	shardPresent[8] = false
	shardPresent[11] = false // Parity 1
	shardPresent[13] = false // Parity 3

	shards[2] = nil
	shards[8] = nil
	shards[11] = nil
	shards[13] = nil

	decoded, err := fec.Decode(shards, shardPresent)
	if err != nil {
		t.Fatalf("Failed to decode with mixed missing shards: %v", err)
	}

	if len(decoded) > len(originalData) {
		decoded = decoded[:len(originalData)]
	}

	if !bytes.Equal(decoded, originalData) {
		t.Errorf("Decoded data doesn't match original")
	}
}

// TestDecodeMaxRecovery tests recovery when exactly parityShards number of shards are missing
func TestDecodeMaxRecovery(t *testing.T) {
	dataShards := 5
	parityShards := 3
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	originalData := make([]byte, 480)
	for i := range originalData {
		originalData[i] = byte((i * 31) % 256)
	}

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}

	// Lose exactly 3 shards
	shardPresent[1] = false
	shardPresent[4] = false
	shardPresent[6] = false // Parity 1

	shards[1] = nil
	shards[4] = nil
	shards[6] = nil

	decoded, err := fec.Decode(shards, shardPresent)
	if err != nil {
		t.Fatalf("Failed to decode with max missing shards: %v", err)
	}

	if len(decoded) > len(originalData) {
		decoded = decoded[:len(originalData)]
	}

	if !bytes.Equal(decoded, originalData) {
		t.Errorf("Decoded data doesn't match original")
	}
}

// TestDecodeExceedRecovery tests that decoding fails when more than parityShards are missing
func TestDecodeExceedRecovery(t *testing.T) {
	dataShards := 5
	parityShards := 3
	shardSize := 100
	fec, err := NewFEC(dataShards, parityShards, shardSize)
	if err != nil {
		t.Fatalf("Failed to create FEC: %v", err)
	}

	originalData := []byte("Test exceed recovery")

	shards, err := fec.Encode(originalData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	shardPresent := make([]bool, len(shards))
	for i := range shardPresent {
		shardPresent[i] = true
	}

	// Lose 4 shards (exceeds parityShards of 3)
	shardPresent[0] = false
	shardPresent[2] = false
	shardPresent[4] = false
	shardPresent[7] = false // Parity 2

	shards[0] = nil
	shards[2] = nil
	shards[4] = nil
	shards[7] = nil

	_, err = fec.Decode(shards, shardPresent)
	if err == nil {
		t.Error("Expected error when missing too many shards, but got nil")
	}
}
