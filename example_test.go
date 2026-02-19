package gexorank_test

import (
	"fmt"

	"github.com/lupppig/gexorank"
)

func ExampleInitial() {
	r := gexorank.Initial()
	fmt.Println(r)
	// Output: 0|iiiiii
}

func ExampleBetween() {
	a, _ := gexorank.Parse("0|aaaaaa")
	b, _ := gexorank.Parse("0|zzzzzz")
	mid, _ := gexorank.Between(a, b)
	fmt.Println(mid)
	// Output: 0|n55554
}

func ExampleGenBetween_append() {
	last, _ := gexorank.Parse("0|iiiiii")
	rank, _ := gexorank.GenBetween(&last, nil)
	fmt.Println(rank)
	// Output: 0|r99998
}

func ExampleGenBetween_prepend() {
	first, _ := gexorank.Parse("0|iiiiii")
	rank, _ := gexorank.GenBetween(nil, &first)
	fmt.Println(rank)
	// Output: 0|999999
}

func ExampleGenBetween_insert() {
	a, _ := gexorank.Parse("0|cccccc")
	b, _ := gexorank.Parse("0|ffffff")
	rank, _ := gexorank.GenBetween(&a, &b)
	fmt.Println(rank)
	// Output: 0|dvvvvv
}

func ExampleLexoRank_GenNext() {
	r := gexorank.Initial()
	next := r.GenNext()
	fmt.Println(next)
	// Output: 0|r99998
}

func ExampleLexoRank_GenPrev() {
	r := gexorank.Initial()
	prev := r.GenPrev()
	fmt.Println(prev)
	// Output: 0|999999
}

func ExampleRebalance() {
	// Build some ranks.
	r1 := gexorank.Initial()
	r2 := r1.GenNext()
	r3 := r2.GenNext()

	ranks := []gexorank.LexoRank{r1, r2, r3}

	// Rebalance into bucket 1 with even spacing.
	rebalanced := gexorank.Rebalance(ranks, gexorank.Bucket1)
	for _, r := range rebalanced {
		fmt.Println(r)
	}
	// Output:
	// 1|8zzzzz
	// 1|hzzzzy
	// 1|qzzzzx
}

func ExampleSort() {
	a, _ := gexorank.Parse("0|zzzzzz")
	b, _ := gexorank.Parse("0|aaaaaa")
	c, _ := gexorank.Parse("0|iiiiii")

	ranks := []gexorank.LexoRank{a, b, c}
	gexorank.Sort(ranks)

	for _, r := range ranks {
		fmt.Println(r)
	}
	// Output:
	// 0|aaaaaa
	// 0|iiiiii
	// 0|zzzzzz
}

func ExampleParse() {
	r, err := gexorank.Parse("2|abc123")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("bucket=%s value=%s\n", r.Bucket(), r.Value())
	// Output: bucket=2 value=abc123
}
