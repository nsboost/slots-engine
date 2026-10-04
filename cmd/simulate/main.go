// cmd/simulate runs a Monte Carlo RTP simulation against a game config.
// This is the tool you re-run after ANY reel strip or paytable change,
// before touching the client. Never ship a math change without a fresh
// RTP report — this is also exactly the kind of report a certification
// lab will ask you to reproduce independently.
package main

import (
	"flag"
	"fmt"
	"math"
	"time"

	"slots-engine/internal/engine"
)

func main() {
	spins := flag.Int("spins", 2_000_000, "number of spins to simulate")
	linesPlayed := flag.Int("lines", 9, "paylines played per spin")
	betPerLine := flag.Int64("bet", 1, "bet per line, in coins")
	flag.Parse()

	cfg := engine.DemoFortuneReels()
	eng, err := engine.New(cfg)
	if err != nil {
		fmt.Println("config error:", err)
		return
	}

	if *linesPlayed > len(cfg.Paylines) {
		*linesPlayed = len(cfg.Paylines)
	}

	var totalBet, totalWin int64
	var winningSpins int64
	var maxWin int64
	winDist := make(map[int64]int64) // win amount (in bet units) -> count, for volatility sketch

	start := time.Now()
	for i := 0; i < *spins; i++ {
		res, err := eng.Spin(engine.SpinRequest{
			SessionID:   "sim",
			BetPerLine:  *betPerLine,
			LinesPlayed: *linesPlayed,
		})
		if err != nil {
			fmt.Println("spin error:", err)
			return
		}
		totalBet += res.TotalBet
		totalWin += res.TotalWin
		if res.TotalWin > 0 {
			winningSpins++
		}
		if res.TotalWin > maxWin {
			maxWin = res.TotalWin
		}
		// Bucket win as multiple of total bet (rounded) for a rough
		// volatility/hit-distribution picture.
		if res.TotalBet > 0 {
			bucket := res.TotalWin / res.TotalBet
			winDist[bucket]++
		}
	}
	elapsed := time.Since(start)

	rtp := float64(totalWin) / float64(totalBet) * 100
	hitFreq := float64(winningSpins) / float64(*spins) * 100

	fmt.Printf("=== %s — RTP Simulation ===\n", cfg.Name)
	fmt.Printf("GameVersion:      %s\n", engine.GameVersion)
	fmt.Printf("Spins:            %d\n", *spins)
	fmt.Printf("Lines played:     %d\n", *linesPlayed)
	fmt.Printf("Bet per line:     %d\n", *betPerLine)
	fmt.Printf("Total bet:        %d\n", totalBet)
	fmt.Printf("Total win:        %d\n", totalWin)
	fmt.Printf("RTP:              %.3f%%\n", rtp)
	fmt.Printf("Hit frequency:    %.2f%%\n", hitFreq)
	fmt.Printf("Max single win:   %d (x%.0f total bet)\n", maxWin, float64(maxWin)/float64(*betPerLine*int64(*linesPlayed)))
	fmt.Printf("Sim time:         %s (%.0f spins/sec)\n", elapsed, float64(*spins)/elapsed.Seconds())

	fmt.Println("\n--- Win size distribution (x total bet per spin) ---")
	printDistribution(winDist, *spins)

	fmt.Println("\nTarget band for social-casino demo: 94-96% RTP.")
	if rtp < 90 || rtp > 98 {
		fmt.Println("WARNING: RTP is well outside the typical social-casino band — re-tune reel weights or paytable.")
	}
}

func printDistribution(dist map[int64]int64, totalSpins int) {
	// Print the most common non-zero buckets, sorted by multiple.
	var maxBucket int64
	for b := range dist {
		if b > maxBucket {
			maxBucket = b
		}
	}
	for b := int64(0); b <= maxBucket; b++ {
		c, ok := dist[b]
		if !ok || c == 0 {
			continue
		}
		pct := float64(c) / float64(totalSpins) * 100
		bar := int(math.Round(pct * 2))
		if bar > 60 {
			bar = 60
		}
		fmt.Printf("  x%-5d  %8d  %6.3f%%  %s\n", b, c, pct, repeat("#", bar))
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
