package fec

import (
	"errors"
)

// GF(2^8) arithmetic tables
var (
	gfExp [512]byte
	gfLog [256]byte
)

func init() {
	// Initialize Galois Field tables using irreducible polynomial 0x11d (x^8 + x^4 + x^3 + x^2 + 1)
	poly := 0x11d
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfExp[i+255] = byte(x)
		gfLog[x] = byte(i)
		x <<= 1
		if x >= 256 {
			x ^= poly
		}
	}
	gfExp[510] = 0
	gfExp[511] = 0
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

func gfDiv(a, b byte) byte {
	if a == 0 {
		return 0
	}
	if b == 0 {
		panic("division by zero")
	}
	return gfExp[(int(gfLog[a])+255-int(gfLog[b]))%255]
}

func gfInv(a byte) byte {
	if a == 0 {
		panic("inverse of zero")
	}
	return gfExp[255-int(gfLog[a])]
}

type matrix [][]byte

func newMatrix(rows, cols int) matrix {
	m := make(matrix, rows)
	for i := range m {
		m[i] = make([]byte, cols)
	}
	return m
}

func buildEncodingMatrix(dataShards, parityShards int) matrix {
	rows := dataShards + parityShards
	cols := dataShards
	m := newMatrix(rows, cols)

	// Top dataShards rows: identity matrix
	for i := 0; i < dataShards; i++ {
		m[i][i] = 1
	}

	// Bottom parityShards rows: Cauchy matrix
	for r := dataShards; r < rows; r++ {
		for c := 0; c < cols; c++ {
			val := byte(r) ^ byte(c)
			m[r][c] = gfInv(val)
		}
	}

	return m
}

func invertMatrix(m matrix) (matrix, error) {
	n := len(m)
	if n == 0 || n != len(m[0]) {
		return nil, errors.New("matrix must be square")
	}

	// Create augmented matrix [m | I]
	aug := newMatrix(n, n*2)
	for i := 0; i < n; i++ {
		copy(aug[i][:n], m[i])
		aug[i][n+i] = 1
	}

	// Gaussian elimination over GF(2^8)
	for i := 0; i < n; i++ {
		// Find pivot
		pivot := i
		for j := i; j < n; j++ {
			if aug[j][i] != 0 {
				pivot = j
				break
			}
		}

		if aug[pivot][i] == 0 {
			return nil, errors.New("singular matrix")
		}

		// Swap rows
		if pivot != i {
			aug[i], aug[pivot] = aug[pivot], aug[i]
		}

		// Normalize pivot row
		invVal := gfInv(aug[i][i])
		for j := i; j < n*2; j++ {
			aug[i][j] = gfMul(aug[i][j], invVal)
		}

		// Eliminate other rows
		for j := 0; j < n; j++ {
			if j != i && aug[j][i] != 0 {
				factor := aug[j][i]
				for k := i; k < n*2; k++ {
					aug[j][k] ^= gfMul(factor, aug[i][k])
				}
			}
		}
	}

	// Extract inverse matrix
	inv := newMatrix(n, n)
	for i := 0; i < n; i++ {
		copy(inv[i], aug[i][n:])
	}
	return inv, nil
}

// FEC implements Forward Error Correction using Reed-Solomon codes
type FEC struct {
	dataShards   int
	parityShards int
	shardSize    int
	encMatrix    matrix
}

// NewFEC creates a new FEC encoder/decoder
func NewFEC(dataShards, parityShards, shardSize int) (*FEC, error) {
	if dataShards <= 0 || parityShards <= 0 {
		return nil, errors.New("dataShards and parityShards must be positive")
	}
	if dataShards+parityShards > 256 {
		return nil, errors.New("total shards cannot exceed 256")
	}
	if shardSize <= 0 {
		return nil, errors.New("shardSize must be positive")
	}

	return &FEC{
		dataShards:   dataShards,
		parityShards: parityShards,
		shardSize:    shardSize,
		encMatrix:    buildEncodingMatrix(dataShards, parityShards),
	}, nil
}

// Encode splits data into shards and generates parity shards
func (f *FEC) Encode(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}

	totalShards := f.dataShards
	shardSize := (len(data) + totalShards - 1) / totalShards

	if f.shardSize > 0 && shardSize < f.shardSize {
		shardSize = f.shardSize
	}

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

	for i := 0; i < f.parityShards; i++ {
		shards[f.dataShards+i] = make([]byte, shardSize)
		for j := 0; j < shardSize; j++ {
			var val byte
			for k := 0; k < f.dataShards; k++ {
				val ^= gfMul(f.encMatrix[f.dataShards+i][k], shards[k][j])
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

	presentCount := 0
	for _, present := range shardPresent {
		if present {
			presentCount++
		}
	}

	if presentCount < f.dataShards {
		return nil, errors.New("not enough shards to reconstruct data")
	}

	var shardSize int
	for i := 0; i < len(shards); i++ {
		if shardPresent[i] && shards[i] != nil && len(shards[i]) > 0 {
			shardSize = len(shards[i])
			break
		}
	}
	if shardSize == 0 {
		return nil, errors.New("no valid shards found to determine shard size")
	}

	// Check if any data shard is missing
	missingData := false
	for i := 0; i < f.dataShards; i++ {
		if !shardPresent[i] {
			missingData = true
			break
		}
	}

	if missingData {
		subMatrix := newMatrix(f.dataShards, f.dataShards)
		subShards := make([][]byte, f.dataShards)

		rowIdx := 0
		for i := 0; i < len(shards) && rowIdx < f.dataShards; i++ {
			if shardPresent[i] {
				copy(subMatrix[rowIdx], f.encMatrix[i])
				subShards[rowIdx] = shards[i]
				rowIdx++
			}
		}

		invMatrix, err := invertMatrix(subMatrix)
		if err != nil {
			return nil, err
		}

		for i := 0; i < f.dataShards; i++ {
			if !shardPresent[i] {
				shards[i] = make([]byte, shardSize)
				for j := 0; j < shardSize; j++ {
					var val byte
					for k := 0; k < f.dataShards; k++ {
						val ^= gfMul(invMatrix[i][k], subShards[k][j])
					}
					shards[i][j] = val
				}
				shardPresent[i] = true
			}
		}
	}

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
