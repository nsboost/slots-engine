# UI: shared look & feel (palette, styleboxes, buttons, labels, toasts).
# Keeping this in one place is what makes a re-skin a one-file change.
class_name UI
extends RefCounted

const GOLD := Color("#ffcf4a")
const GOLD_LIGHT := Color("#ffe27a")
const GOLD_DARK := Color("#c98a12")
const PANEL := Color(0.09, 0.04, 0.21, 0.94)
const TEXT := Color("#ffffff")
const MUTED := Color("#b9a8e8")
const GREEN := Color("#3ddc84")
const RED := Color("#ff5a6e")


static func box(color: Color, radius: int = 16, border: Color = Color(0, 0, 0, 0), border_w: int = 0, shadow: int = 0) -> StyleBoxFlat:
	var s := StyleBoxFlat.new()
	s.bg_color = color
	s.set_corner_radius_all(radius)
	if border_w > 0:
		s.border_color = border
		s.set_border_width_all(border_w)
	if shadow > 0:
		s.shadow_size = shadow
		s.shadow_color = Color(0, 0, 0, 0.45)
		s.shadow_offset = Vector2(0, shadow * 0.35)
	return s


static func button(text: String, kind: String = "gold", font_size: int = 28, min_size: Vector2 = Vector2(0, 76)) -> Button:
	var b := Button.new()
	b.text = text
	b.custom_minimum_size = min_size
	b.focus_mode = Control.FOCUS_NONE
	b.add_theme_font_size_override("font_size", font_size)

	var base := GOLD
	var edge := GOLD_DARK
	var fg := Color("#3a1a00")
	match kind:
		"purple":
			base = Color("#5b34c4"); edge = Color("#2d1670"); fg = TEXT
		"green":
			base = Color("#2fc46f"); edge = Color("#17803f"); fg = Color("#04210f")
		"red":
			base = Color("#e0475b"); edge = Color("#8f1f30"); fg = TEXT
		"ghost":
			base = Color(1, 1, 1, 0.10); edge = Color(1, 1, 1, 0.28); fg = TEXT

	var radius := int(min_size.y * 0.5) if kind == "spin" else 22
	var states := {
		"normal": base,
		"hover": base.lightened(0.08),
		"pressed": base.darkened(0.14),
		"disabled": Color(base.r * 0.45, base.g * 0.45, base.b * 0.45, base.a * 0.8),
	}
	for state in states:
		var sb := box(states[state], radius, edge, 3, 8 if state != "pressed" else 2)
		sb.set_content_margin_all(12)
		b.add_theme_stylebox_override(state, sb)
	for c in ["font_color", "font_hover_color", "font_pressed_color", "font_focus_color"]:
		b.add_theme_color_override(c, fg)
	b.add_theme_color_override("font_disabled_color", Color(fg.r, fg.g, fg.b, 0.45))
	b.pressed.connect(func(): Sfx.play("click"))
	return b


static func label(text: String, size: int = 24, color: Color = TEXT, align: HorizontalAlignment = HORIZONTAL_ALIGNMENT_LEFT, outline: int = 0) -> Label:
	var l := Label.new()
	l.text = text
	l.horizontal_alignment = align
	l.add_theme_font_size_override("font_size", size)
	l.add_theme_color_override("font_color", color)
	if outline > 0:
		l.add_theme_constant_override("outline_size", outline)
		l.add_theme_color_override("font_outline_color", Color(0, 0, 0, 0.65))
	return l


static func panel(color: Color = PANEL, radius: int = 24, border: Color = Color(1, 1, 1, 0.12), border_w: int = 2, pad: int = 18) -> PanelContainer:
	var p := PanelContainer.new()
	var sb := box(color, radius, border, border_w, 10)
	sb.set_content_margin_all(pad)
	p.add_theme_stylebox_override("panel", sb)
	return p


static func spacer(h: float = 0.0, w: float = 0.0) -> Control:
	var c := Control.new()
	c.custom_minimum_size = Vector2(w, h)
	c.mouse_filter = Control.MOUSE_FILTER_IGNORE
	return c


## 12345678 -> "12,345,678"
static func fmt(n: int) -> String:
	var s := str(absi(n))
	var out := ""
	var count := 0
	for i in range(s.length() - 1, -1, -1):
		out = s[i] + out
		count += 1
		if count % 3 == 0 and i != 0:
			out = "," + out
	return ("-" if n < 0 else "") + out


static func toast(host: Control, msg: String, color: Color = TEXT) -> void:
	var p := panel(Color(0.05, 0.02, 0.12, 0.96), 26, GOLD, 2, 16)
	p.add_child(label(msg, 24, color, HORIZONTAL_ALIGNMENT_CENTER))
	p.mouse_filter = Control.MOUSE_FILTER_IGNORE
	host.add_child(p)
	p.set_anchors_preset(Control.PRESET_CENTER_BOTTOM)
	p.grow_horizontal = Control.GROW_DIRECTION_BOTH
	p.grow_vertical = Control.GROW_DIRECTION_BEGIN
	p.offset_bottom = -150
	p.modulate.a = 0.0
	var tw := p.create_tween()
	tw.tween_property(p, "modulate:a", 1.0, 0.15)
	tw.tween_interval(1.9)
	tw.tween_property(p, "modulate:a", 0.0, 0.35)
	tw.tween_callback(p.queue_free)
