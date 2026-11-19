# gexorank

[![Go Reference](https://pkg.go.dev/badge/github.com/lupppig/gexorank.svg)](https://pkg.go.dev/github.com/lupppig/gexorank)
[![Go Report Card](https://goreportcard.com/badge/github.com/lupppig/gexorank)](https://goreportcard.com/report/github.com/lupppig/gexorank)

A production-grade [LexoRank](https://en.wikipedia.org/wiki/Lexicographic_order) implementation in Go.  
Reorder items in a list by updating **one row** instead of re-indexing the entire table.

## Features

- Zero dependencies
- Immutable & concurrency-safe
- Appends/prepends stay short (~840k in a row before a rank grows past 6 chars)
- Ranks sort identically in Go and in your database (byte-wise)
- Exhaustion is always reported as an error — never a duplicate or out-of-order rank
- Three-bucket rebalancing

## Install

```bash
go get github.com/lupppig/gexorank
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/lupppig/gexorank"
)

func main() {
    // Create the first rank
    first := gexorank.Initial() // "0|iiiiii"

    // Append after it
    second, _ := gexorank.GenBetween(&first, nil) // "0|iiijii"

    // Insert between
    middle, _ := gexorank.GenBetween(&first, &second) // midpoint

    fmt.Println(first)  // 0|iiiiii
    fmt.Println(middle) // 0|iiij0i
    fmt.Println(second) // 0|iiijii
}
```

## API

### Core Functions

| Function | Description |
|---|---|
| `Initial()` | First rank in bucket 0 (midpoint of space) |
| `Min()` | `"0\|000000"` — nothing can be generated before it |
| `Max()` | `"0\|zzzzzz"` — largest 6-char rank (longer ranks can follow it) |
| `Parse(s)` | Parse & validate a rank string like `"0\|abc123"` |
| `Between(a, b)` | Midpoint between two ranks (same bucket) |
| `After(r)` / `Before(r)` | Rank after / before `r`, or `ErrRankExhausted` |
| `GenBetween(prev, next)` | **Recommended.** Nil-safe insert: prepend, append, or between |
| `InsertBetween(...)` / `InsertBetweenFunc(...)` | Read → compute → write with retry on unique-constraint conflicts |
| `Rebalance(ranks, bucket)` | Redistribute ranks evenly into a target bucket |
| `Sort(ranks)` | Sort a slice of LexoRanks in ascending order |

### Methods on `LexoRank`

| Method | Description |
|---|---|
| `GenNext()` / `GenPrev()` | **Deprecated** — use `After` / `Before`. Panics if the space is exhausted |
| `IsZero()` | True for an unset (zero-value) rank |
| `Bucket()` | Returns the bucket (0, 1, or 2) |
| `RankString()` | Raw rank value without bucket prefix |
| `String()` | Full string: `"{bucket}\|{value}"` |
| `CompareTo(other)` | Returns -1, 0, or 1 |
| `InNextBucket()` | Same value in the next bucket |
| `InPrevBucket()` | Same value in the previous bucket |
| `Len()` | Length of the rank value (grows with convergence) |
| `MaxLen()` | Maximum allowed length (128) before exhaustion |
| `NeedsRebalance(t)` | True if `Len() >= t * MaxLen()` (e.g. `t=0.75`) |

LexoRank also implements `database/sql.Scanner`, `driver.Valuer`, `json.Marshaler`, `json.Unmarshaler`, `encoding.TextMarshaler`, and `encoding.TextUnmarshaler` — it works seamlessly with GORM, sqlx, and JSON APIs. A zero-value rank is stored as SQL `NULL` / JSON `null`, and reads back as the zero value.

### Errors

| Error | Meaning |
|---|---|
| `ErrRankExhausted` | No rank fits at the requested position within 128 chars — run `Rebalance` |
| `ErrConflict` | Return (or wrap) this from an `InsertFunc` to signal a duplicate rank |
| `ErrMaxRetriesExceeded` | `InsertBetween` hit a conflict on every attempt |

## `GenBetween` — The One Function You Need

Most use cases map to a single function with nil-safe pointers:

```go
// Empty list → first item
rank, _ := gexorank.GenBetween(nil, nil)

// Prepend (insert at top)
rank, _ = gexorank.GenBetween(nil, &firstRank)

// Append (insert at bottom)
rank, _ = gexorank.GenBetween(&lastRank, nil)

// Insert between two items
rank, _ = gexorank.GenBetween(&prevRank, &nextRank)
```

## Database Integration

LexoRank values are plain strings ordered **byte-wise**. Store them in a column with a binary collation so `ORDER BY` matches `CompareTo` (PostgreSQL `COLLATE "C"`, MySQL `utf8mb4_bin`, SQLite's default `BINARY`):

```sql
CREATE TABLE tasks (
    id      BIGSERIAL PRIMARY KEY,
    list_id BIGINT NOT NULL,
    title   TEXT NOT NULL,
    rank    VARCHAR(256) COLLATE "C" NOT NULL,
    UNIQUE (list_id, rank) -- also serves as the ordering index
);

-- Query in order:
SELECT * FROM tasks WHERE list_id = $1 ORDER BY rank ASC;
```

### GORM Example

```go
type Task struct {
    ID    uint      `gorm:"primaryKey"`
    Title string    `gorm:"not null"`
    Rank  gexorank.LexoRank `gorm:"not null;uniqueIndex;type:varchar(256) collate \"C\""`
}

// Append a new task
var last Task
db.Order("rank DESC").First(&last)

newRank, _ := gexorank.GenBetween(&last.Rank, nil)

db.Create(&Task{Title: "New task", Rank: newRank})
```

See [`examples/gorm/main.go`](examples/gorm/main.go) for a full example.

## Concurrency

The rank computation itself is thread-safe (immutable types, no shared state). However, the **workflow** — read neighbors → compute rank → write — is not atomic. Two concurrent inserts between the same two items will produce **identical ranks**, corrupting sort order.

### `InsertBetween` — The Safe Way

Use the built-in retry helper. You provide two callbacks, the library handles the rest:

```go
rank, err := gexorank.InsertBetween(
    func() (*gexorank.LexoRank, *gexorank.LexoRank, error) {
        var prev, next Task
        db.Where("id = ?", prevID).First(&prev)
        db.Where("id = ?", nextID).First(&next)
        return &prev.Rank, &next.Rank, nil
    },
    func(rank gexorank.LexoRank) error {
        err := db.Create(&Task{Title: "New", Rank: rank}).Error
        if errors.Is(err, gorm.ErrDuplicatedKey) { // needs gorm.Config{TranslateError: true}
            return fmt.Errorf("%w: %w", gexorank.ErrConflict, err)
        }
        return err
    },
    3, // max attempts
)
```

Only errors wrapping `gexorank.ErrConflict` are retried; any other error (e.g. a lost connection) is returned immediately. Alternatively, pass your own classifier:

```go
rank, err := gexorank.InsertBetweenFunc(neighbors, insert,
    func(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }, 3)
```

> **Requires** a `UNIQUE` constraint on the rank column so concurrent duplicates trigger a retry.

If you need more control, two manual patterns are available:

### Option A: Pessimistic Locking (SELECT … FOR UPDATE)

Lock the neighbor rows so only one transaction can insert between them at a time.

```go
tx := db.Begin()

// Lock the two neighbors
var prev, next Task
tx.Clauses(clause.Locking{Strength: "UPDATE"}).
    Where("id IN ?", []uint{prevID, nextID}).
    Order("rank ASC").
    Find(&[]Task{prev, next})

newRank, _ := gexorank.GenBetween(&prev.Rank, &next.Rank)
tx.Create(&Task{Title: "New", Rank: newRank})

tx.Commit()
```

**Pros:** Simple, deterministic.
**Cons:** Holds locks, serializes concurrent inserts in the same region.

### Option B: Optimistic Concurrency (UNIQUE constraint + retry)

Add a unique constraint on `rank` and retry on conflict.

```sql
ALTER TABLE tasks ADD CONSTRAINT uq_tasks_rank UNIQUE (rank);
```

```go
const maxRetries = 3

func InsertBetween(db *gorm.DB, prev, next *gexorank.LexoRank, title string) error {
    for range maxRetries {
        newRank, err := gexorank.GenBetween(prev, next)
        if err != nil {
            return err
        }

        err = db.Create(&Task{Title: title, Rank: newRank}).Error
        if err == nil {
            return nil
        }
        if !errors.Is(err, gorm.ErrDuplicatedKey) {
            return err // not a conflict — don't retry
        }

        // Conflict — re-read neighbors and retry
        // (the winner's insert shifted the gap)
        prev, next = refreshNeighbors(db)
    }
    return fmt.Errorf("rank insert failed after %d retries", maxRetries)
}
```

**Pros:** No row locks, higher throughput.
**Cons:** Retry logic, slightly more code.

### Which to choose?

| Scenario | Recommendation |
|---|---|
| Low concurrency / simple app | **Pessimistic** — less code, good enough |
| High concurrency / real-time collaboration | **Optimistic** — better throughput |
| Bulk import | Neither — use `Rebalance` to assign all ranks at once |

## Rebalancing

When ranks are inserted repeatedly between the same two neighbors, the rank strings grow longer (about one character per 5 inserts into the same gap). When no rank fits within `MaxLength` (128 chars), `Between`, `After`, `Before` and `GenBetween` return `ErrRankExhausted`. Appends and prepends are cheap: about 840,000 in a row fit in 6 characters.

### Monitoring

Detect rank growth before it becomes a problem:

```go
// After every insert, check the new rank
if newRank.NeedsRebalance(0.75) {
    log.Warnf("rank %q is at %d/%d chars, consider rebalancing",
        newRank, newRank.Len(), newRank.MaxLen())
}
```

**Recovery pattern:**

```go
mid, err := gexorank.GenBetween(&prev, &next)
if errors.Is(err, gexorank.ErrRankExhausted) {
    // Fetch all ranks, rebalance into the next bucket
    allRanks := fetchAllRanksSorted()
    currentBucket := allRanks[0].Bucket()
    fresh := gexorank.Rebalance(allRanks, currentBucket.Next())

    // Bulk update in a transaction
    updateAllRanks(fresh)
}
```

The three-bucket rotation (`0→1→2→0`) lets you write new ranks to an inactive bucket while reads continue on the active one — no downtime.

## Benchmarks

```
goos: linux
goarch: amd64
cpu: Intel(R) Core(TM) i7-7600U CPU @ 2.80GHz

BenchmarkParse-4           11119063       107.4 ns/op      32 B/op    1 allocs/op
BenchmarkBetween-4           629113      1649 ns/op       264 B/op   12 allocs/op
BenchmarkAfter-4             700866      2322 ns/op       352 B/op   13 allocs/op
BenchmarkBefore-4            672405      1793 ns/op       208 B/op   10 allocs/op
BenchmarkRebalance100-4       25782     43050 ns/op      9249 B/op  309 allocs/op
```

Run locally: `go test -bench=. -benchmem ./...`

## License

MIT
