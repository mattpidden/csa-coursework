package main

import (
	"fmt"
	"os"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

// Change the following constant to change how many turns to GoL benchmark should iterate
const benchLength = 5000

func BenchmarkStudentVersion(b *testing.B) {
	imageSizes := []int{16, 64, 128, 256, 512}
	//for threads := 1; threads <= 1; threads++ {
	for _, size := range imageSizes {
		os.Stdout = nil // Disable all program output apart from benchmark results
		p := gol.Params{
			Turns:       benchLength,
			Threads:     1,
			ImageWidth:  size,
			ImageHeight: size,
		}

		name := fmt.Sprintf("%dx%dx%d-%d", p.ImageWidth, p.ImageHeight, p.Turns, p.Threads)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				events := make(chan gol.Event)
				go gol.Run(p, events, nil)
				for range events {
				}
			}
		})
	}
}
