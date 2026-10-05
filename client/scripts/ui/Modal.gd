# Modal: dimmed overlay with a centered titled panel. Fill `body`.
class_name Modal
extends Control

signal closed

var body: VBoxContainer
var _panel: PanelContainer


static func open(host: Control, title: String, width: int = 600) -> Modal:
	var m := Modal.new()
	host.add_child(m)
	m._build(title, width)
	return m


func _build(title: String, width: int) -> void:
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_STOP

	var dim := ColorRect.new()
	dim.color = Color(0, 0, 0, 0.7)
	dim.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(dim)

	var center := CenterContainer.new()
	center.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	center.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(center)

	_panel = UI.panel(Color(0.11, 0.05, 0.26, 0.98), 32, UI.GOLD, 3, 28)
	_panel.custom_minimum_size = Vector2(width, 0)
	center.add_child(_panel)

	var v := VBoxContainer.new()
	v.add_theme_constant_override("separation", 18)
	_panel.add_child(v)
	v.add_child(UI.label(title, 38, UI.GOLD, HORIZONTAL_ALIGNMENT_CENTER, 3))
	body = VBoxContainer.new()
	body.add_theme_constant_override("separation", 14)
	v.add_child(body)

	_panel.modulate.a = 0.0
	create_tween().tween_property(_panel, "modulate:a", 1.0, 0.18)


func add_close_button(text: String = "Close", kind: String = "purple") -> Button:
	var b := UI.button(text, kind, 28)
	b.pressed.connect(close)
	body.add_child(b)
	return b


func close() -> void:
	closed.emit()
	queue_free()
