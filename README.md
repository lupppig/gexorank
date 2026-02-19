# gexorank

[![Go Reference](https://pkg.go.dev/badge/github.com/lupppig/gexorank.svg)](https://pkg.go.dev/github.com/lupppig/gexorank)
[![Go Report Card](https://goreportcard.com/badge/github.com/lupppig/gexorank)](https://goreportcard.com/report/github.com/lupppig/gexorank)

A production-grade [LexoRank](https://en.wikipedia.org/wiki/Lexicographic_order) implementation in Go.  
Reorder items in a list by updating **one row** instead of re-indexing the entire table.

## Features

- ero dependencies
- Immutable & concurrent
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
    second, _ := gexorank.GenBetween(&first, nil) // "0|iiiiiii"

    // Insert between
    middle, _ := gexorank.GenBetween(&first, &second) // midpoint

    fmt.Println(first)  // 0|iiiiii
    fmt.Println(middle) // 0|iiiiii9
    fmt.Println(second) // 0|iiiiiii
}
```

## API

### Core Functions

| Function | Description |
|---|---|
| `Initial()` | First rank in bucket 0 (midpoint of space) |
| `Min()` | Minimum possible rank in bucket 0 |
| `Max()` | Maximum possible rank in bucket 0 |
| `Parse(s)` | Parse & validate a rank string like `"0\|abc123"` |
| `Between(a, b)` | Midpoint between two ranks (same bucket) |
| `GenBetween(prev, next)` | **Recommended.** Nil-safe insert: prepend, append, or between |
| `Rebalance(ranks, bucket)` | Redistribute ranks evenly into a target bucket |
| `Sort(ranks)` | Sort a slice of LexoRanks in ascending order |

### Methods on `LexoRank`

| Method | Description |
|---|---|
| `GenNext()` | Rank after this one |
| `GenPrev()` | Rank before this one |
| `Bucket()` | Returns the bucket (0, 1, or 2) |
| `RankString()` | Raw rank value without bucket prefix |
| `String()` | Full string: `"{bucket}\|{value}"` |
| `CompareTo(other)` | Returns -1, 0, or 1 |
| `InNextBucket()` | Same value in the next bucket |
| `InPrevBucket()` | Same value in the previous bucket |
| `Len()` | Length of the rank value (grows with convergence) |
| `MaxLen()` | Maximum allowed length (128) before exhaustion |
| `NeedsRebalance(t)` | True if `Len() >= t * MaxLen()` (e.g. `t=0.75`) |

LexoRank also implements `database/sql.Scanner`, `driver.Valuer`, `json.Marshaler`, `json.Unmarshaler`, `encoding.TextMarshaler`, and `encoding.TextUnmarshaler` — it works seamlessly with GORM, sqlx, and JSON APIs.

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

LexoRank values are plain strings. Store them in a `VARCHAR` or `TEXT` column with an index:

```sql
CREATE TABLE tasks (
    id    BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    rank  VARCHAR(256) NOT NULL
);

CREATE INDEX idx_tasks_rank ON tasks (rank);

-- Query in order:
SELECT * FROM tasks ORDER BY rank ASC;
```

### GORM Example

```go
type Task struct {
    ID    uint      `gorm:"primaryKey"`
    Title string    `gorm:"not null"`
    Rank  gexorank.LexoRank `gorm:"not null;index;type:varchar(256)"`
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
        return db.Create(&Task{Title: "New", Rank: rank}).Error
    },
    3, // max retries
)
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

        result := db.Create(&Task{Title: title, Rank: newRank})
        if result.Error == nil {
            return nil
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

When ranks are inserted repeatedly between the same two neighbors, the rank strings grow longer. When they exceed `MaxLength` (128 chars), `Between` returns `ErrRankExhausted`.

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
mid, err := gexorank.Between(a, b)
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

BenchmarkParse-4            22131580        53.32 ns/op      32 B/op    1 allocs/op
BenchmarkBetween-4           1327888       899.9 ns/op      264 B/op   12 allocs/op
BenchmarkGenNext-4             67490     15624 ns/op        910 B/op   10 allocs/op
BenchmarkGenPrev-4             46514     24085 ns/op        910 B/op   10 allocs/op
BenchmarkRebalance100-4        30820     35825 ns/op      10872 B/op  513 allocs/op
```

Run locally: `go test -bench=. -benchmem ./...`

## License

MIT
