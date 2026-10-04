// cmd/play runs single spins against the demo config and prints the grid
// and any wins, for manual sanity-checking of the engine output shape
// before it's wired into an API.
package main

import (
	"flag"
	"fmt"

	"slots-engine/internal/engine"
)

func main() {
	gameID := flag.String("game", "fortune-reels", "game id to play")
	n := flag.Int("n", 5, "number of spins to print")
	flag.Parse()

	cfg, ok := engine.GameByID(*gameID)
	if !ok {
		fmt.Println("unknown game:", *gameID)
		return
	}
	eng, err := engine.New(cfg)
	if err != nil {
		fmt.Println("config error:", err)
		return
	}

	var balance int64 = 1000
	for i := 0; i < *n; i++ {
		res, err := eng.Spin(engine.SpinRequest{
			SessionID:   "manual",
			BetPerLine:  1,
			LinesPlayed: 9,
		})
		if err != nil {
			fmt.Println("spin error:", err)
			return
		}
		balance += res.TotalWin - res.TotalBet

		fmt.Printf("\n--- Spin %d ---\n", i+1)
		printGrid(res.Grid, cfg.Rows)
		if len(res.LineWins) == 0 && res.ScatterWin == 0 {
			fmt.Println("No win.")
		}
		for _, w := range res.LineWins {
			fmt.Printf("Line %d: %d x %s -> %d\n", w.LineIndex, w.Count, w.Symbol, w.Win)
		}
		if res.ScatterWin > 0 {
			fmt.Printf("Scatter win: %d\n", res.ScatterWin)
		}
		fmt.Printf("Bet: %d  Win: %d  Balance: %d\n", res.TotalBet, res.TotalWin, balance)
	}
}

func printGrid(grid [][]engine.Symbol, rows int) {
	for r := 0; r < rows; r++ {
		for c := 0; c < len(grid); c++ {
			fmt.Printf("%-8s", grid[c][r])
		}
		fmt.Println()
	}
}
