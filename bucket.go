// Package gexorank implements the LexoRank algorithm for efficient list ordering.
//
// This file defines the Bucket type used by LexoRank's three-bucket rebalancing system.
package gexorank

import "fmt"

// Bucket represents one of the three LexoRank buckets (0, 1, 2).
// Buckets enable background rebalancing without disrupting active ranking.
type Bucket uint8

const (
	// Bucket0 is the first bucket.
	Bucket0 Bucket = 0
	// Bucket1 is the second bucket.
	Bucket1 Bucket = 1
	// Bucket2 is the third bucket.
	Bucket2 Bucket = 2

	bucketCount = 3
)

// Next returns the next bucket in the rotation (0→1→2→0).
func (b Bucket) Next() Bucket {
	return (b + 1) % bucketCount
}

// Prev returns the previous bucket in the rotation (0→2→1→0).
func (b Bucket) Prev() Bucket {
	return (b + bucketCount - 1) % bucketCount
}

// String returns the string representation of the bucket ("0", "1", or "2").
func (b Bucket) String() string {
	return fmt.Sprintf("%d", b)
}

// ParseBucket parses a single-character string into a Bucket.
// It returns an error if the input is not "0", "1", or "2".
func ParseBucket(s string) (Bucket, error) {
	switch s {
	case "0":
		return Bucket0, nil
	case "1":
		return Bucket1, nil
	case "2":
		return Bucket2, nil
	default:
		return 0, fmt.Errorf("gexorank: invalid bucket %q, must be 0, 1, or 2", s)
	}
}
