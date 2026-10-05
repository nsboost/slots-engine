# SymbolView: procedurally drawn slot symbol (no art assets). Each symbol
# code from the server ("S10","SJ","SQ","SK","SA","B1","B2","WILD",
# "SCATTER") maps to a color + glyph. Swapping in real art later means
# replacing _draw() with a TextureRect — nothing else in the reel grid
# changes, since callers only ever call set_code().
class_name SymbolView
extends Control

static var _font: Font = ThemeDB.fallback_font

var code: String = "S10"
var dimmed: bool = false
var highlighted: bool = false

const INFO := {
	"S10":     {"glyph": "10", "fg": Color("#dfe7ff"), "bg": Color("#2b3a66")},
	"SJ":      {"glyph": "J",  "fg": Color("#dfe7ff"), "bg": Color("#2b3a66")},
	"SQ":      {"glyph": "Q",  "fg": Color("#ffe3f3"), "bg": Color("#6a2b55")},
	"SK":      {"glyph": "K",  "fg": Color("#fff3d6"), "bg": Color("#6a4b16")},
	"SA":      {"glyph": "A",  "fg": Color("#e9fff0"), "bg": Color("#1f6a3f")},
	"B1":      {"glyph": "♣",  "fg": Color("#fff7df"), "bg": Color("#8a3b12")},
	"B2":      {"glyph": "♦",  "fg": Color("#2a1300"), "bg": Color("#ffb33e")},
	"WILD":    {"glyph": "W",  "fg": Color("#2a1300"), "bg": Color("#ffd24a")},
	"SCATTER": {"glyph": "★",  "fg": Color("#1b0b33"), "bg": Color("#9b6bff")},
}


func set_code(c: String) -> void:
	code = c
	queue_redraw()


func _draw() -> void:
	var info: Dictionary = INFO.get(code, INFO["S10"])
	var bg: Color = info.bg
	if dimmed:
		bg = bg.darkened(0.55)
	var r := Rect2(Vector2.ZERO, size)
	draw_rect(r, bg, true, -1.0, true)
	var border_col: Color = Color("#ffe27a") if highlighted else Color(1, 1, 1, 0.14)
	draw_rect(r, border_col, false, (3.0 if highlighted else 1.5), true)

	var glyph: String = info.glyph
	var fg: Color = Color("#2a1300") if highlighted else info.fg
	var fs := int(size.y * 0.46)
	var w := _font.get_string_size(glyph, HORIZONTAL_ALIGNMENT_CENTER, -1, fs).x
	draw_string(_font, Vector2((size.x - w) / 2.0, size.y * 0.68), glyph, HORIZONTAL_ALIGNMENT_LEFT, -1, fs, fg)

	if code == "WILD" or code == "SCATTER" or code == "B2":
		draw_rect(r, Color(1, 1, 0.75, 0.10 if not highlighted else 0.0), true, -1.0, true)


func set_highlighted(v: bool) -> void:
	highlighted = v
	queue_redraw()
