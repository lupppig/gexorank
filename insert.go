package gexorank

import "fmt"

// ErrMaxRetriesExceeded is returned when [InsertBetween] exhausts all retry
// attempts without a successful insert.
var ErrMaxRetriesExceeded = fmt.Errorf("gexorank: max retries exceeded")

// NeighborFunc returns the prev and next ranks surrounding the insert position.
// Either pointer may be nil (prepend/append). It is called before each attempt,
// so it must re-read from the database to reflect any concurrent changes.
type NeighborFunc func() (prev, next *LexoRank, err error)

// InsertFunc attempts to persist a row with the given rank. It should return a
// non-nil error when the insert fails due to a unique constraint violation
// (duplicate rank). Any other error is treated as fatal and stops the retry loop.
type InsertFunc func(rank LexoRank) error

// InsertBetween performs the read-compute-write cycle with automatic retry on
// rank conflicts. On each attempt it:
//  1. Calls neighbors to get the current prev/next ranks.
//  2. Computes a new rank via [GenBetween].
//  3. Calls insert with the computed rank.
//
// If insert returns an error, the cycle restarts (up to maxRetries total
// attempts). If all attempts fail, [ErrMaxRetriesExceeded] is returned.
//
// The caller is responsible for adding a UNIQUE constraint on the rank column
// so that concurrent duplicate inserts cause a conflict error.
//
// Example (GORM):
//
//	rank, err := gexorank.InsertBetween(
//	    func() (*gexorank.LexoRank, *gexorank.LexoRank, error) {
//	        var prev, next Task
//	        // ... read neighbors from DB ...
//	        return &prev.Rank, &next.Rank, nil
//	    },
//	    func(rank gexorank.LexoRank) error {
//	        return db.Create(&Task{Title: "New", Rank: rank}).Error
//	    },
//	    3,
//	)
func InsertBetween(neighbors NeighborFunc, insert InsertFunc, maxRetries int) (LexoRank, error) {
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error
	for range maxRetries {
		prev, next, err := neighbors()
		if err != nil {
			return LexoRank{}, fmt.Errorf("gexorank: neighbors: %w", err)
		}

		rank, err := GenBetween(prev, next)
		if err != nil {
			return LexoRank{}, fmt.Errorf("gexorank: gen rank: %w", err)
		}

		if err := insert(rank); err != nil {
			lastErr = err
			continue
		}

		return rank, nil
	}

	return LexoRank{}, fmt.Errorf("%w: last error: %v", ErrMaxRetriesExceeded, lastErr)
}
