// Package main demonstrates integrating gexorank with GORM.
//
// This example shows how to use LexoRank as a sortable column
// in a database-backed model. The rank is stored as a plain string
// column, and the application uses gexorank for rank calculations.
//
// This is NOT part of the gexorank library — it's a standalone example
// that imports gexorank as a dependency.
package main

import (
	"fmt"
	"sort"

	"github.com/lupppig/gexorank"
)

// Task represents a sortable to-do item.
// In a real GORM application, this struct would have gorm tags.
//
//	type Task struct {
//	    ID    uint   `gorm:"primaryKey"`
//	    Title string `gorm:"not null"`
//	    Rank  string `gorm:"not null;index"`
//	}
type Task struct {
	ID    uint
	Title string
	Rank  string // stored as a plain string, e.g. "0|iiiiii"
}

// LexoRank parses the stored rank string into a gexorank.LexoRank.
func (t Task) LexoRank() (gexorank.LexoRank, error) {
	return gexorank.Parse(t.Rank)
}

func main() {
	// --- Creating the first task ---
	// When the list is empty, use Initial().
	first := gexorank.Initial()
	tasks := []Task{
		{ID: 1, Title: "Buy groceries", Rank: first.String()},
	}
	fmt.Println("1. Created first task:")
	printTasks(tasks)

	// --- Appending a task ---
	// When adding to the end, use GenBetween(lastRank, nil).
	lastRank, _ := tasks[len(tasks)-1].LexoRank()
	appendRank, _ := gexorank.GenBetween(&lastRank, nil)
	tasks = append(tasks, Task{ID: 2, Title: "Walk the dog", Rank: appendRank.String()})
	fmt.Println("2. Appended task:")
	printTasks(tasks)

	// --- Prepending a task ---
	// When adding to the beginning, use GenBetween(nil, firstRank).
	firstRank, _ := tasks[0].LexoRank()
	prependRank, _ := gexorank.GenBetween(nil, &firstRank)
	tasks = append(tasks, Task{ID: 3, Title: "Morning coffee", Rank: prependRank.String()})
	fmt.Println("3. Prepended task:")
	printTasks(tasks)

	// --- Inserting between two tasks ---
	// Use GenBetween(prevRank, nextRank) to insert between.
	prevRank, _ := gexorank.Parse(tasks[0].Rank) // "Buy groceries"
	nextRank, _ := gexorank.Parse(tasks[1].Rank) // "Walk the dog"
	betweenRank, _ := gexorank.GenBetween(&prevRank, &nextRank)
	tasks = append(tasks, Task{ID: 4, Title: "Read a book", Rank: betweenRank.String()})
	fmt.Println("4. Inserted between:")
	printTasks(tasks)

	// --- Querying in order ---
	// In GORM: db.Order("rank ASC").Find(&tasks)
	// Here we simulate with sort:
	sortTasks(tasks)
	fmt.Println("5. Sorted by rank:")
	printTasks(tasks)
}

func sortTasks(tasks []Task) {
	sort.Slice(tasks, func(i, j int) bool {
		ri, _ := gexorank.Parse(tasks[i].Rank)
		rj, _ := gexorank.Parse(tasks[j].Rank)
		return ri.CompareTo(rj) < 0
	})
}

func printTasks(tasks []Task) {
	for _, t := range tasks {
		fmt.Printf("  [%d] %-20s rank=%s\n", t.ID, t.Title, t.Rank)
	}
	fmt.Println()
}
