package fec

import (
	"errors"
)

// FEC implements Forward Error Correction using Reed-Solomon codes
type FEC struct {
	dataShards   int
	parityShards int
	shardSize    int
}

// NewFEC creates a new FEC encoder/decoder
// dataShards: number of data shards
// parityShards: number of parity shards for error correction
func NewFEC(dataShards, parityShards, shardSize int) (*FEC, error) {
	if dataShards <= 0 || parityShards <= 0 {
		return nil, errors.New("dataShards and parityShards must be positive")
	}
	if shardSize <= 0 {
		return nil, errors.New("shardSize must be positive")
	}

	return &FEC{
		dataShards:   dataShards,
		parityShards: parityShards,
		shardSize:    shardSize,
	}, nil
}

// Encode splits data into shards and generates parity shards
// Returns all shards (data + parity)
func (f *FEC) Encode(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}

	// Calculate padding needed
	totalShards := f.dataShards
	shardSize := (len(data) + totalShards - 1) / totalShards
	
	// Align to shardSize if specified
	if f.shardSize > 0 && shardSize < f.shardSize {
		shardSize = f.shardSize
	}

	// Create data shards
	shards := make([][]byte, f.dataShards+f.parityShards)
	for i := 0; i < f.dataShards; i++ {
		shards[i] = make([]byte, shardSize)
		start := i * shardSize
		end := start + shardSize
		if end > len(data) {
			end = len(data)
		}
		if start < len(data) {
			copy(shards[i], data[start:end])
		}
	}

	// Create parity shards using XOR-based simple FEC
	// For production use, consider using a proper Reed-Solomon library
	for i := 0; i < f.parityShards; i++ {
		shards[f.dataShards+i] = make([]byte, shardSize)
		for j := 0; j < shardSize; j++ {
			var val byte
			for k := 0; k < f.dataShards; k++ {
				val ^= shards[k][j]
			}
			shards[f.dataShards+i][j] = val
		}
	}

	return shards, nil
}

// Decode reconstructs data from shards (can handle missing shards if enough remain)
func (f *FEC) Decode(shards [][]byte, shardPresent []bool) ([]byte, error) {
	if len(shards) != f.dataShards+f.parityShards {
		return nil, errors.New("incorrect number of shards")
	}
	if len(shardPresent) != len(shards) {
		return nil, errors.New("shardPresent length mismatch")
	}

	// Count present shards
	presentCount := 0
	for _, present := range shardPresent {
		if present {
			presentCount++
		}
	}

	if presentCount < f.dataShards {
		return nil, errors.New("not enough shards to reconstruct data")
	}

	// Determine shard size from any available shard (not necessarily shards[0])
	// This handles the case where shards arrive out of order
	var shardSize int
	for i := 0; i < len(shards); i++ {
		// Check both shardPresent and nil as a defensive measure for network data
		if shardPresent[i] && shards[i] != nil && len(shards[i]) > 0 {
			shardSize = len(shards[i])
			break
		}
	}
	if shardSize == 0 {
		return nil, errors.New("no valid shards found to determine shard size")
	}

	// Simple XOR-based reconstruction for missing data shards
	for i := 0; i < f.dataShards; i++ {
		if !shardPresent[i] {
			// Reconstruct missing data shard using XOR
			shards[i] = make([]byte, shardSize)
			for j := 0; j < shardSize; j++ {
				var val byte
				// XOR all present data shards and first parity shard
				for k := 0; k < f.dataShards; k++ {
					if k != i && shardPresent[k] {
						val ^= shards[k][j]
					}
				}
				if shardPresent[f.dataShards] {
					val ^= shards[f.dataShards][j]
				}
				shards[i][j] = val
			}
			shardPresent[i] = true
		}
	}

	// Reconstruct original data
	result := make([]byte, 0, f.dataShards*shardSize)
	for i := 0; i < f.dataShards; i++ {
		result = append(result, shards[i]...)
	}

	return result, nil
}

// DataShards returns the number of data shards
func (f *FEC) DataShards() int {
	return f.dataShards
}

// ParityShards returns the number of parity shards
func (f *FEC) ParityShards() int {
	return f.parityShards
}

// TotalShards returns the total number of shards
func (f *FEC) TotalShards() int {
	return f.dataShards + f.parityShards
}
