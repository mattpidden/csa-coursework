package main

import (
	"fmt"
	"os"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

// Change the following constant to change how many turns to GoL benchmark should iterate
const benchLength = 1000

func BenchmarkStudentVersion(b *testing.B) {
	for threads := 1; threads <= 16; threads++ {
		os.Stdout = nil // Disable all program output apart from benchmark results
		p := gol.Params{
			Turns:       benchLength,
			Threads:     threads,
			ImageWidth:  512,
			ImageHeight: 512,
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



/*
// Benchmark applies the gol.Run to the 512x512 image, for 1000 turns, b.N times.
// The time taken is carefully measured by go.
// The b.N  repetition is needed because benchmark results are not always constant.

func TestBenchmarkGol(b *testing.B) {
	// Disable all program output apart from benchmark results
	//os.Stdout = nil

	// Use a for-loop to run 5 sub-benchmarks, with 1, 2, 4, 8 and 16 worker threads.
	for threads := 1; threads <= 16; threads*=2 {
		b.Run(fmt.Sprintf("%d_workers", threads), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var params gol.Params
				params.ImageWidth = 512
				params.ImageHeight = 512
				params.Turns = 0
				params.Threads = threads

				keyPresses := make(chan rune, 10)
				events := make(chan gol.Event, 1000)

				gol.Run(params, events, keyPresses)
				//filter("ship.png", fmt.Sprintf("out_%d.png", threads), threads)
			}
		})
	}
}
*/