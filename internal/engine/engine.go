// Package engine implements server-authoritative slot machine math.
//
// Design principles (do not relax these without re-reading /areas/slots-game.md):
//   - The server is the only source of truth for spin outcomes. Clients
//     NEVER compute results locally — they only animate what the server
//     returns. This protects the math from tampering and is a hard
//     requirement for eventual lab certification (GLI/BMM) and for any
//     real-money mode.
//   - RNG seeding uses crypto/rand, not math/rand, because a predictable
//     seed is a certification failure and a cheating vector.
//   - All monetary/coin amounts are integers (smallest unit = 1 coin).
//     Never use floating point for balances — only for RTP/statistics
//     reporting.
//   - GameVersion is embedded in every spin result so that math changes
//     are auditable and can be pinned per-session if ever required by a
//     regulator.
package engine

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// GameVersion identifies this build of the math model. Bump it whenever
// reel strips, paytable, or paylines change. Certification labs test a
// specific pinned GameVersion — never silently change math in place.
const GameVersion = "0.1.0-demo"

// Symbol is a single reel symbol id.
type Symbol string

const (
	SymWild    Symbol = "WILD"
	SymScatter Symbol = "SCATTER"
	Sym10      Symbol = "S10"
	SymJ       Symbol = "SJ"
	SymQ       Symbol = "SQ"
	SymK       Symbol = "SK"
	SymA       Symbol = "SA"
	SymBonus1  Symbol = "B1" // low-tier bonus symbol
	SymBonus2  Symbol = "B2" // high-tier bonus symbol
)

// Config bundles everything that defines a game's math. It is the unit
// that gets versioned, simulated, and eventually certified.
type Config struct {
	Name        string
	Rows        int
	Reels       [][]Symbol // Reels[col] = full strip for that column, in strip order
	Paytable    Paytable
	Paylines    []Payline // each payline is one Symbol index per column (row index)
	WildSymbol  Symbol
	ScatterPays ScatterPaytable
}

// Payline is a row index per reel column (left to right).
type Payline []int

// PayRule maps "N of this symbol in a line" -> payout multiplier (of bet-per-line).
type PayRule map[int]int // count -> multiplier

// Paytable maps symbol -> PayRule. Only symbols that pay on lines go here.
type Paytable map[Symbol]PayRule

// ScatterPaytable maps scatter count -> multiplier of TOTAL bet (not per-line).
type ScatterPaytable map[int]int

// SpinRequest is what a client sends the server to request a spin.
// The server never trusts anything else from the client about the outcome.
type SpinRequest struct {
	SessionID   string
	BetPerLine  int64
	LinesPlayed int // must be <= len(Config.Paylines)
}

// SpinResult is the full, signed-off outcome of one spin. This is the
// only thing a client is allowed to render.
type SpinResult struct {
	GameVersion string
	Grid        [][]Symbol // Grid[col][row]
	LineWins    []LineWin
	ScatterWin  int64
	TotalWin    int64
	TotalBet    int64
	Balance     int64 // caller fills in after applying TotalBet/TotalWin to the ledger
}

// LineWin describes a single winning payline.
type LineWin struct {
	LineIndex int
	Symbol    Symbol
	Count     int
	Win       int64
}

// Engine runs spins against a fixed Config. One Engine per game.
type Engine struct {
	cfg Config
}

func New(cfg Config) (*Engine, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &Engine{cfg: cfg}, nil
}

func validateConfig(cfg Config) error {
	if cfg.Rows <= 0 {
		return fmt.Errorf("rows must be > 0")
	}
	if len(cfg.Reels) == 0 {
		return fmt.Errorf("no reels defined")
	}
	for i, strip := range cfg.Reels {
		if len(strip) < cfg.Rows {
			return fmt.Errorf("reel %d shorter than visible rows", i)
		}
	}
	for i, pl := range cfg.Paylines {
		if len(pl) != len(cfg.Reels) {
			return fmt.Errorf("payline %d has wrong column count", i)
		}
		for _, r := range pl {
			if r < 0 || r >= cfg.Rows {
				return fmt.Errorf("payline %d row index out of range", i)
			}
		}
	}
	return nil
}

// secureRandIndex returns a cryptographically secure random index in [0,n).
func secureRandIndex(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("n must be > 0")
	}
	bi, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(bi.Int64()), nil
}

// Spin runs one spin server-side. bet is validated by the caller (ledger
// layer) BEFORE calling this — Engine does not touch balances.
func (e *Engine) Spin(req SpinRequest) (SpinResult, error) {
	if req.LinesPlayed <= 0 || req.LinesPlayed > len(e.cfg.Paylines) {
		return SpinResult{}, fmt.Errorf("invalid lines played: %d", req.LinesPlayed)
	}
	if req.BetPerLine <= 0 {
		return SpinResult{}, fmt.Errorf("bet per line must be > 0")
	}

	grid := make([][]Symbol, len(e.cfg.Reels))
	for col, strip := range e.cfg.Reels {
		stopIdx, err := secureRandIndex(len(strip))
		if err != nil {
			return SpinResult{}, fmt.Errorf("rng failure: %w", err)
		}
		// Take Rows consecutive symbols starting at stopIdx, wrapping around
		// the strip. This is the standard "reel stop" model.
		col_syms := make([]Symbol, e.cfg.Rows)
		for r := 0; r < e.cfg.Rows; r++ {
			col_syms[r] = strip[(stopIdx+r)%len(strip)]
		}
		grid[col] = col_syms
	}

	result := SpinResult{
		GameVersion: GameVersion,
		Grid:        grid,
		TotalBet:    req.BetPerLine * int64(req.LinesPlayed),
	}

	// Evaluate paylines actually played (first N in config order — client
	// and server must agree on payline ordering via GameVersion).
	for i := 0; i < req.LinesPlayed; i++ {
		line := e.cfg.Paylines[i]
		win := e.evalLine(grid, line, req.BetPerLine)
		if win.Win > 0 {
			win.LineIndex = i
			result.LineWins = append(result.LineWins, win)
			result.TotalWin += win.Win
		}
	}

	// Scatter pays apply across the whole grid regardless of lines played,
	// scaled by total bet.
	scatterCount := 0
	for _, col := range grid {
		for _, s := range col {
			if s == SymScatter {
				scatterCount++
			}
		}
	}
	if mult, ok := e.cfg.ScatterPays[scatterCount]; ok {
		result.ScatterWin = result.TotalBet * int64(mult)
		result.TotalWin += result.ScatterWin
	}

	return result, nil
}

// evalLine checks one payline for a win, counting from the left and
// allowing SymWild to substitute for any paying symbol.
func (e *Engine) evalLine(grid [][]Symbol, line Payline, betPerLine int64) LineWin {
	if len(line) == 0 {
		return LineWin{}
	}
	first := grid[0][line[0]]
	target := first
	if target == e.cfg.WildSymbol {
		// Wild-only start: find the first non-wild further along to
		// determine the paying symbol, if any.
		for col := 1; col < len(line); col++ {
			s := grid[col][line[col]]
			if s != e.cfg.WildSymbol {
				target = s
				break
			}
		}
	}
	if target == SymScatter {
		return LineWin{} // scatters never pay on lines
	}
	rule, ok := e.cfg.Paytable[target]
	if !ok {
		return LineWin{}
	}

	count := 0
	for col := 0; col < len(line); col++ {
		s := grid[col][line[col]]
		if s == target || s == e.cfg.WildSymbol {
			count++
		} else {
			break
		}
	}

	mult, ok := rule[count]
	if !ok || mult <= 0 {
		return LineWin{}
	}
	return LineWin{Symbol: target, Count: count, Win: betPerLine * int64(mult)}
}
