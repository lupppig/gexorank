package gexorank

import (
	"errors"
	"fmt"
)

// ErrMaxRetriesExceeded is returned when [InsertBetween] exhausts all retry
// attempts without a successful insert.
var ErrMaxRetriesExceeded = errors.New("gexorank: max retries exceeded")

// ErrConflict signals that an insert failed because the rank is already taken
// (a unique constraint violation). Return it, or an error wrapping it, from an
// [InsertFunc] to make [InsertBetween] retry with fresh neighbours.
var ErrConflict = errors.New("gexorank: rank conflict")

// NeighborFunc returns the prev and next ranks surrounding the insert position.
// Either pointer may be nil (prepend/append). It is called before each attempt,
// so it must re-read from the database to reflect any concurrent changes.
type NeighborFunc func() (prev, next *LexoRank, err error)

// InsertFunc attempts to persist a row with the given rank.
type InsertFunc func(rank LexoRank) error

// InsertBetween performs the read-compute-write cycle with automatic retry on
// rank conflicts. On each attempt it:
//  1. Calls neighbors to get the current prev/next ranks.
//  2. Computes a new rank via [GenBetween].
//  3. Calls insert with the computed rank.
//
// If insert returns an error wrapping [ErrConflict], the cycle restarts (up to
// maxRetries total attempts); if all attempts conflict, the returned error
// wraps both [ErrMaxRetriesExceeded] and the last conflict error. Any other
// error from insert is returned immediately without retrying.
//
// The caller is responsible for adding a UNIQUE constraint on the rank column
// so that concurrent duplicate inserts cause a conflict error.
//
// Example (GORM, with TranslateError enabled):
//
//	rank, err := gexorank.InsertBetween(
//	    func() (*gexorank.LexoRank, *gexorank.LexoRank, error) {
//	        // ... re-read prev/next from DB; nil means "no neighbour" ...
//	        return prev, next, nil
//	    },
//	    func(rank gexorank.LexoRank) error {
//	        err := db.Create(&Task{Title: "New", Rank: rank}).Error
//	        if errors.Is(err, gorm.ErrDuplicatedKey) {
//	            return fmt.Errorf("%w: %w", gexorank.ErrConflict, err)
//	        }
//	        return err
//	    },
//	    3,
//	)
//
// Use [InsertBetweenFunc] to classify conflicts with your own function instead.
func InsertBetween(neighbors NeighborFunc, insert InsertFunc, maxRetries int) (LexoRank, error) {
	return InsertBetweenFunc(neighbors, insert, isErrConflict, maxRetries)
}

// InsertBetweenFunc is like [InsertBetween] but uses isConflict to decide
// which insert errors are rank conflicts that should be retried. This lets you
// pass your driver's unique-violation check directly, e.g.
//
//	func(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }
//
// A nil isConflict treats only errors wrapping [ErrConflict] as conflicts.
func InsertBetweenFunc(neighbors NeighborFunc, insert InsertFunc, isConflict func(error) bool, maxRetries int) (LexoRank, error) {
	if isConflict == nil {
		isConflict = isErrConflict
	}
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

		err = insert(rank)
		if err == nil {
			return rank, nil
		}
		if !isConflict(err) {
			return LexoRank{}, fmt.Errorf("gexorank: insert: %w", err)
		}
		lastErr = err
	}

	return LexoRank{}, fmt.Errorf("%w after %d attempts: %w", ErrMaxRetriesExceeded, maxRetries, lastErr)
}

func isErrConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}
