package engine

// DemoFortuneReels is a 5-column, 3-row demo game config, loosely in the
// style of a classic fruit/jackpot-themed social slot (comparable in
// structure to things like Crown Coins Casino's simpler titles — NOT a
// copy of their math, which is proprietary and unknown to us).
//
// Reel strip weighting is the primary RTP lever: a symbol's frequency on
// the strip is just how many times it appears. These strips were weighted
// by hand to land near a 94-96% target RTP band (typical social-casino
// range) and MUST be re-verified with cmd/simulate after any change —
// never ship a strip change without re-running the simulator.
func DemoFortuneReels() Config {
	// Strip composition per column. Higher-paying symbols appear less
	// often; low symbols (10/J/Q/K/A) are the filler. Columns are allowed
	// to differ slightly to avoid a too-predictable near-miss pattern.
	lowHeavy := []Symbol{
		Sym10, SymJ, SymQ, SymK, SymA,
		Sym10, SymJ, SymQ, SymK, SymA,
		Sym10, SymJ, SymQ, SymK,
		SymBonus1, SymBonus1,
		SymBonus2,
		SymWild,
		SymScatter,
		Sym10, SymJ, SymQ, SymK, SymA,
	}
	col3 := []Symbol{
		Sym10, SymJ, SymQ, SymK, SymA,
		Sym10, SymJ, SymQ, SymK, SymA,
		SymBonus1, SymBonus1, SymBonus1,
		SymBonus2, SymBonus2,
		SymWild,
		SymScatter, SymScatter,
		Sym10, SymJ, SymQ, SymK, SymA,
	}

	reels := [][]Symbol{lowHeavy, lowHeavy, col3, lowHeavy, lowHeavy}

	paytable := Paytable{
		Sym10:     PayRule{3: 4, 4: 10, 5: 30},
		SymJ:      PayRule{3: 5, 4: 15, 5: 45},
		SymQ:      PayRule{3: 7, 4: 22, 5: 70},
		SymK:      PayRule{3: 9, 4: 36, 5: 135},
		SymA:      PayRule{3: 14, 4: 60, 5: 270},
		SymBonus1: PayRule{3: 18, 4: 88, 5: 440},
		SymBonus2: PayRule{3: 44, 4: 220, 5: 1750},
		SymWild:   PayRule{3: 53, 4: 265, 5: 2650},
	}

	scatterPays := ScatterPaytable{
		3: 4,
		4: 17,
		5: 85,
	}

	// 9 paylines across a 5x3 grid, row indices per column (0=top,1=mid,2=bottom).
	paylines := []Payline{
		{1, 1, 1, 1, 1}, // middle
		{0, 0, 0, 0, 0}, // top
		{2, 2, 2, 2, 2}, // bottom
		{0, 1, 2, 1, 0}, // V
		{2, 1, 0, 1, 2}, // inverted V
		{1, 0, 0, 0, 1},
		{1, 2, 2, 2, 1},
		{0, 0, 1, 2, 2},
		{2, 2, 1, 0, 0},
	}

	return Config{
		Name:        "Fortune Reels (demo)",
		Rows:        3,
		Reels:       reels,
		Paytable:    paytable,
		Paylines:    paylines,
		WildSymbol:  SymWild,
		ScatterPays: scatterPays,
	}
}
