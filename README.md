# gexorank

[![Go Reference](https://pkg.go.dev/badge/github.com/lupppig/gexorank.svg)](https://pkg.go.dev/github.com/lupppig/gexorank)
[![Go Report Card](https://goreportcard.com/badge/github.com/lupppig/gexorank)](https://goreportcard.com/report/github.com/lupppig/gexorank)

A production-grade [LexoRank](https://en.wikipedia.org/wiki/Lexicographic_order) implementation in Go.  
Reorder items in a list by updating **one row** instead of re-indexing the entire table.

## Features

- **Zero dependencies** — only Go standard library (`math/big`, `strings`, `errors`)
- **Immutable & concurrent** — no mutexes needed, safe to share across goroutines
- **Three-bucket rebalancing** — handles rank exhaustion gracefully
- **Base36 encoding** — compact, sortable strings (`0-9a-z`)
- **Canonical form** — fixed-width, zero-padded values guarantee `ORDER BY rank ASC` correctness

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
    second, _ := gexorank.GenBetween(&first, nil) // "0|r99998"

    // Insert between
    middle, _ := gexorank.GenBetween(&first, &second) // midpoint

    fmt.Println(first)  // 0|iiiiii
    fmt.Println(middle) // 0|n22226
    fmt.Println(second) // 0|r99998
}
```

## API

### Core Functions

| Function | Description |
|---|---|
| `Initial()` | First rank in bucket 0 (midpoint of space) |
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
| `Value()` | Raw rank value without bucket prefix |
| `String()` | Full string: `"{bucket}\|{value}"` |
| `CompareTo(other)` | Returns -1, 0, or 1 |
| `InNextBucket()` | Same value in the next bucket |
| `InPrevBucket()` | Same value in the previous bucket |

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
    ID    uint   `gorm:"primaryKey"`
    Title string `gorm:"not null"`
    Rank  string `gorm:"not null;index;size:256"`
}

// Append a new task
var last Task
db.Order("rank DESC").First(&last)

lastRank, _ := gexorank.Parse(last.Rank)
newRank, _ := gexorank.GenBetween(&lastRank, nil)

db.Create(&Task{Title: "New task", Rank: newRank.String()})
```

See [`examples/gorm/main.go`](examples/gorm/main.go) for a full example.

## Rebalancing

When ranks are inserted repeatedly between the same two neighbors, the rank strings grow longer. When they exceed `MaxLength` (128 chars), `Between` returns `ErrRankExhausted`.

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

BenchmarkParse-4            22262826        58.93 ns/op      32 B/op    1 allocs/op
BenchmarkBetween-4           1000000      1016 ns/op        264 B/op   12 allocs/op
BenchmarkGenNext-4             37693     30167 ns/op       1807 B/op   19 allocs/op
BenchmarkGenPrev-4            422378      2887 ns/op        388 B/op    7 allocs/op
BenchmarkRebalance100-4        32313     33943 ns/op      10872 B/op  513 allocs/op
```

Run locally: `go test -bench=. -benchmem ./...`

## License

MIT
