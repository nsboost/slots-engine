# ReelGrid: the 5x3 (cols x rows) symbol grid, including the spin
# animation. Deliberately simple physics (no literal spinning reel strip
# texture) — each column free-spins through random symbols, columns stop
# left-to-right with a small stagger and an overshoot/settle bounce, then
# winning-line cells pulse gold. Enough to feel alive without needing
# texture assets or a physics-accurate reel strip.
class_name ReelGrid
extends Control

signal spin_finished

const COLS := 5
const ROWS := 3
const CELL := Vector2(66, 66)
const GAP := 7
const SYMBOL_POOL := ["S10", "SJ", "SQ", "SK", "SA", "B1", "B2", "WILD", "SCATTER"]

var _cells: Array = []  # flat, index = row*COLS+col
var _spinning: bool = false
var _rng := RandomNumberGenerator.new()


func _ready() -> void:
	custom_minimum_size = Vector2(COLS * CELL.x + (COLS - 1) * GAP, ROWS * CELL.y + (ROWS - 1) * GAP)
	var frame := UI.panel(Color(0, 0, 0, 0.35), 20, Color(1, 1, 1, 0.10), 2, GAP)
	add_child(frame)
	var grid := GridContainer.new()
	grid.columns = COLS
	grid.add_theme_constant_override("h_separation", GAP)
	grid.add_theme_constant_override("v_separation", GAP)
	frame.add_child(grid)

	_cells.clear()
	for i in COLS * ROWS:
		var sv := SymbolView.new()
		sv.custom_minimum_size = CELL
		sv.set_code(SYMBOL_POOL[i % SYMBOL_POOL.size()])
		grid.add_child(sv)
		_cells.append(sv)


func cell(col: int, row: int) -> SymbolView:
	return _cells[row * COLS + col]


func clear_highlights() -> void:
	for c in _cells:
		c.set_highlighted(false)


## grid_data is server shape: grid[col] = [row0, row1, row2] symbol codes.
func spin_to(grid_data: Array) -> void:
	if _spinning:
		return
	_spinning = true
	clear_highlights()
	Sfx.play("spin_start")

	var tl := create_tween()
	tl.set_parallel(true)
	for col in COLS:
		var delay := col * 0.11
		var spin_time := 0.45 + col * 0.12
		tl.tween_callback(_flicker_column.bind(col, spin_time)).set_delay(delay)

	var total := 0.45 + (COLS - 1) * 0.12 + 0.55
	await get_tree().create_timer(total).timeout

	for col in range(grid_data.size()):
		var col_syms: Array = grid_data[col]
		for row in range(col_syms.size()):
			cell(col, row).set_code(str(col_syms[row]))
		_bounce_column(col)
		Sfx.play("reel_stop")

	await get_tree().create_timer(0.18).timeout
	_spinning = false
	spin_finished.emit()


func _flicker_column(col: int, duration: float) -> void:
	var elapsed := 0.0
	var step := 0.045
	while elapsed < duration:
		for row in ROWS:
			cell(col, row).set_code(SYMBOL_POOL[_rng.randi_range(0, SYMBOL_POOL.size() - 1)])
		await get_tree().create_timer(step).timeout
		elapsed += step


func _bounce_column(col: int) -> void:
	for row in ROWS:
		var c := cell(col, row)
		c.pivot_offset = CELL / 2.0
		c.scale = Vector2(1.0, 0.72)
		var tw := c.create_tween()
		tw.tween_property(c, "scale", Vector2(1.04, 1.04), 0.10).set_trans(Tween.TRANS_QUAD).set_ease(Tween.EASE_OUT)
		tw.tween_property(c, "scale", Vector2.ONE, 0.09).set_trans(Tween.TRANS_QUAD).set_ease(Tween.EASE_IN)


## line_wins: server shape [{lineIndex, symbol, count, win}]; paylines:
## server GameInfo.paylines (list of per-column row indices). Highlights
## only the first `count` columns of each winning line, matching how the
## server evaluates left-to-right runs.
func highlight_wins(line_wins: Array, paylines: Array) -> void:
	clear_highlights()
	for w in line_wins:
		var li := int(w.get("lineIndex", -1))
		var count := int(w.get("count", 0))
		if li < 0 or li >= paylines.size():
			continue
		var line: Array = paylines[li]
		for col in range(mini(count, line.size())):
			cell(col, int(line[col])).set_highlighted(true)


func is_spinning() -> bool:
	return _spinning
