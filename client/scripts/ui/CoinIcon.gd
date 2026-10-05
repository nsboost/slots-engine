class_name CoinIcon
extends Control


func _init(sz: float = 44.0) -> void:
	custom_minimum_size = Vector2(sz, sz)
	size = Vector2(sz, sz)
	mouse_filter = Control.MOUSE_FILTER_IGNORE


func _draw() -> void:
	var c := size / 2.0
	var r := minf(size.x, size.y) / 2.0 - 1.0
	draw_circle(c, r, UI.GOLD_DARK)
	draw_circle(c, r * 0.90, UI.GOLD)
	draw_circle(c, r * 0.66, UI.GOLD_LIGHT)
	draw_arc(c, r * 0.66, 0.0, TAU, 32, UI.GOLD_DARK, maxf(1.5, r * 0.08), true)
	var fs := int(r * 1.05)
	var font := ThemeDB.fallback_font
	draw_string(font, Vector2(0.0, c.y + fs * 0.35), "$", HORIZONTAL_ALIGNMENT_CENTER, size.x, fs, Color("#8a4a00"))
