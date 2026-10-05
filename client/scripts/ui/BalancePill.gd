# BalancePill: coin icon + animated balance, bound to Session.
class_name BalancePill
extends PanelContainer

var _label: Label
var _shown: float = 0.0
var _tween: Tween


func _ready() -> void:
	var sb := UI.box(Color(0, 0, 0, 0.45), 32, UI.GOLD_DARK, 2)
	sb.content_margin_left = 12
	sb.content_margin_right = 20
	sb.content_margin_top = 6
	sb.content_margin_bottom = 6
	add_theme_stylebox_override("panel", sb)

	var h := HBoxContainer.new()
	h.add_theme_constant_override("separation", 10)
	add_child(h)
	h.add_child(CoinIcon.new(40.0))
	_label = UI.label("0", 30, UI.TEXT)
	_label.custom_minimum_size = Vector2(110, 0)
	h.add_child(_label)

	_shown = float(Session.display_total)
	_label.text = UI.fmt(int(_shown))
	Session.balance_changed.connect(_on_changed)


func _on_changed() -> void:
	if _tween:
		_tween.kill()
	_tween = create_tween()
	_tween.tween_method(_set_shown, _shown, float(Session.display_total), 0.55) \
		.set_trans(Tween.TRANS_QUAD).set_ease(Tween.EASE_OUT)


func _set_shown(v: float) -> void:
	_shown = v
	_label.text = UI.fmt(int(round(v)))
