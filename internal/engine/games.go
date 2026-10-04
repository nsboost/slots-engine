package engine

// AllGames returns every game the platform offers, in lobby order. Adding
// a game means: define a Config here, add it to this list, run
// cmd/simulate -game=<id> until RTP is in band, and add a client theme in
// client/scripts/ui/GameThemes.gd. Nothing else changes.
func AllGames() []Config {
	return []Config{DemoFortuneReels(), GoldRush()}
}

// GameByID looks up a game config by its ID.
func GameByID(id string) (Config, bool) {
	for _, g := range AllGames() {
		if g.ID == id {
			return g, true
		}
	}
	return Config{}, false
}

// GoldRush is the second game: 15 paylines and a higher-volatility
// paytable (rarer, bigger hits) than Fortune Reels. Same symbol set so the
// client can reuse symbol art, different reel composition and payouts so
// it plays differently. RTP verified with cmd/simulate -game=gold-rush.
func GoldRush() Config {
	outer := []Symbol{
		Sym10, SymJ, SymQ, SymK, SymA,
		Sym10, SymJ, SymQ, SymK, SymA,
		Sym10, SymJ, SymQ, SymK, SymA,
		Sym10, SymJ, SymQ, SymK, SymA,
		SymBonus1, SymBonus1, SymBonus1,
		SymBonus2, SymBonus2,
		SymWild,
		SymScatter, SymScatter,
	}
	// Reels 1 and 5 carry no wild (wilds on the outermost reels inflate
	// RTP disproportionately); reels 2-4 do.
	edge := make([]Symbol, len(outer))
	copy(edge, outer)
	for i, sym := range edge {
		if sym == SymWild {
			edge[i] = SymQ
		}
	}
	middle := []Symbol{
		Sym10, SymJ, SymQ, SymK, SymA,
		Sym10, SymJ, SymQ, SymK, SymA,
		Sym10, SymJ, SymQ, SymK, SymA,
		SymBonus1, SymBonus1, SymBonus1, SymBonus1,
		SymBonus2, SymBonus2, SymBonus2,
		SymWild, Sym10,
		SymScatter, SymScatter, SymScatter,
	}

	return Config{
		ID:         "gold-rush",
		Name:       "Gold Rush",
		Rows:       3,
		Reels:      [][]Symbol{edge, outer, middle, outer, edge},
		WildSymbol: SymWild,
		Paytable: Paytable{
			Sym10:     PayRule{3: 1, 4: 3, 5: 10},
			SymJ:      PayRule{3: 2, 4: 4, 5: 12},
			SymQ:      PayRule{3: 2, 4: 6, 5: 18},
			SymK:      PayRule{3: 3, 4: 10, 5: 35},
			SymA:      PayRule{3: 4, 4: 12, 5: 50},
			SymBonus1: PayRule{3: 7, 4: 32, 5: 160},
			SymBonus2: PayRule{3: 20, 4: 100, 5: 800},
			SymWild:   PayRule{3: 30, 4: 150, 5: 1500},
		},
		ScatterPays: ScatterPaytable{3: 1, 4: 5, 5: 30},
		Paylines: []Payline{
			{1, 1, 1, 1, 1}, {0, 0, 0, 0, 0}, {2, 2, 2, 2, 2},
			{0, 1, 2, 1, 0}, {2, 1, 0, 1, 2},
			{0, 0, 1, 2, 2}, {2, 2, 1, 0, 0},
			{1, 0, 0, 0, 1}, {1, 2, 2, 2, 1},
			{0, 1, 1, 1, 0}, {2, 1, 1, 1, 2},
			{1, 0, 1, 2, 1}, {1, 2, 1, 0, 1},
			{0, 1, 0, 1, 0}, {2, 1, 2, 1, 2},
		},
	}
}
