package engine

import "testing"

func TestDemoConfigValidates(t *testing.T) {
	cfg := DemoFortuneReels()
	if _, err := New(cfg); err != nil {
		t.Fatalf("demo config failed validation: %v", err)
	}
}

func TestRejectsBadConfig(t *testing.T) {
	cfg := DemoFortuneReels()
	cfg.Rows = 0
	if _, err := New(cfg); err == nil {
		t.Fatal("expected error for zero rows, got nil")
	}
}

func TestSpinRejectsInvalidBet(t *testing.T) {
	eng, _ := New(DemoFortuneReels())
	_, err := eng.Spin(SpinRequest{BetPerLine: 0, LinesPlayed: 1})
	if err == nil {
		t.Fatal("expected error for zero bet per line")
	}
}

func TestSpinRejectsInvalidLines(t *testing.T) {
	eng, _ := New(DemoFortuneReels())
	_, err := eng.Spin(SpinRequest{BetPerLine: 1, LinesPlayed: 0})
	if err == nil {
		t.Fatal("expected error for zero lines played")
	}
	_, err = eng.Spin(SpinRequest{BetPerLine: 1, LinesPlayed: 999})
	if err == nil {
		t.Fatal("expected error for too many lines played")
	}
}

// TestNoNegativeWins is a cheap regression guard: across many spins, no
// win should ever be negative and TotalWin must equal the sum of its
// parts. This would catch an integer overflow or logic inversion bug
// long before it reached a simulation run.
func TestSpinInvariants(t *testing.T) {
	eng, err := New(DemoFortuneReels())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50_000; i++ {
		res, err := eng.Spin(SpinRequest{BetPerLine: 1, LinesPlayed: 9})
		if err != nil {
			t.Fatalf("spin %d failed: %v", i, err)
		}
		if res.TotalWin < 0 {
			t.Fatalf("spin %d: negative total win %d", i, res.TotalWin)
		}
		sum := res.ScatterWin
		for _, w := range res.LineWins {
			if w.Win <= 0 {
				t.Fatalf("spin %d: non-positive recorded line win %d", i, w.Win)
			}
			sum += w.Win
		}
		if sum != res.TotalWin {
			t.Fatalf("spin %d: line+scatter wins (%d) != TotalWin (%d)", i, sum, res.TotalWin)
		}
		if res.TotalBet != 9 {
			t.Fatalf("spin %d: expected bet 9, got %d", i, res.TotalBet)
		}
	}
}

// TestRTPInBand is a fast guard against gross math regressions (e.g.
// someone fat-fingers a paytable multiplier by 10x). It uses far fewer
// spins than cmd/simulate and a looser band, so it won't flake in CI —
// it is not a substitute for running cmd/simulate before any real
// paytable/reel change.
func TestRTPRoughlyInBand(t *testing.T) {
	eng, err := New(DemoFortuneReels())
	if err != nil {
		t.Fatal(err)
	}
	var bet, win int64
	const spins = 500_000
	for i := 0; i < spins; i++ {
		res, err := eng.Spin(SpinRequest{BetPerLine: 1, LinesPlayed: 9})
		if err != nil {
			t.Fatal(err)
		}
		bet += res.TotalBet
		win += res.TotalWin
	}
	rtp := float64(win) / float64(bet) * 100
	if rtp < 88 || rtp > 102 {
		t.Fatalf("RTP %.2f%% is far outside sane bounds — check for a paytable/reel regression", rtp)
	}
}
