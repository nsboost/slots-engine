# Backdrop: vertical gradient with softly twinkling stars.
class_name Backdrop
extends Control

var top: Color = Color("#2c1068")
var bottom: Color = Color("#0c0524")
var _stars: Array = []
var _t: float = 0.0
var _accum: float = 0.0


func _ready() -> void:
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_IGNORE
	var rng := RandomNumberGenerator.new()
	rng.seed = 20261004
	for i in 48:
		_stars.append([Vector2(rng.randf(), rng.randf()), rng.randf_range(1.2, 3.4), rng.randf() * TAU])


func set_colors(t: Color, b: Color) -> void:
	top = t
	bottom = b
	queue_redraw()


func _process(delta: float) -> void:
	_t += delta
	_accum += delta
	if _accum >= 0.066:  # ~15 fps is plenty for a slow twinkle
		_accum = 0.0
		queue_redraw()


func _draw() -> void:
	var pts := PackedVector2Array([Vector2.ZERO, Vector2(size.x, 0), size, Vector2(0, size.y)])
	draw_polygon(pts, PackedColorArray([top, top, bottom, bottom]))
	for s in _stars:
		var a := 0.22 + 0.38 * (0.5 + 0.5 * sin(_t * 1.3 + float(s[2])))
		draw_circle((s[0] as Vector2) * size, float(s[1]), Color(1.0, 0.95, 0.8, a))
