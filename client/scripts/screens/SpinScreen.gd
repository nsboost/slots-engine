# SpinScreen: the core gameplay loop for one game. Bet controls, the reel
# grid, spin button, win display. Talks to the server exclusively through
# Session.api.spin — this client never computes an outcome itself (see the
# server-side engine package doc comment for why that's non-negotiable).
class_name SpinScreen
extends Control

signal back_pressed
signal open_store
signal low_balance_during_play

var game_id: String
var game_info: Dictionary
var reels: ReelGrid
var bet_label: Label
var win_label: Label
var spin_button: Button
var status_label: Label
var backdrop: Backdrop


func setup(id: String) -> void:
	game_id = id
	game_info = Session.game_by_id(id)
	_build()


func _build() -> void:
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	var theme: Dictionary = GameThemes.for_game(game_id)

	backdrop = Backdrop.new()
	backdrop.set_colors(theme.bg_top, theme.bg_bottom)
	add_child(backdrop)

	var margin := MarginContainer.new()
	margin.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	for side in ["margin_left", "margin_right", "margin_top", "margin_bottom"]:
		margin.add_theme_constant_override(side, 18)
	add_child(margin)

	var root := VBoxContainer.new()
	root.add_theme_constant_override("separation", 14)
	margin.add_child(root)

	root.add_child(_top_bar(theme))

	var reel_center := CenterContainer.new()
	reel_center.size_flags_vertical = Control.SIZE_EXPAND_FILL
	root.add_child(reel_center)
	reels = ReelGrid.new()
	reel_center.add_child(reels)
	reels.spin_finished.connect(_on_spin_finished)

	win_label = UI.label("Spin to play!", 22, UI.TEXT, HORIZONTAL_ALIGNMENT_CENTER)
	root.add_child(win_label)

	root.add_child(_bet_bar())

	status_label = UI.label("", 14, UI.MUTED, HORIZONTAL_ALIGNMENT_CENTER)
	root.add_child(status_label)


func _top_bar(theme: Dictionary) -> HBoxContainer:
	var bar := HBoxContainer.new()
	bar.add_theme_constant_override("separation", 10)

	var back := UI.button("< Back", "ghost", 16, Vector2(90, 56))
	back.pressed.connect(func(): back_pressed.emit())
	bar.add_child(back)

	var title := UI.label(str(game_info.get("name", game_id)), 24, theme.accent, HORIZONTAL_ALIGNMENT_LEFT, 2)
	title.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	bar.add_child(title)

	bar.add_child(BalancePill.new())

	var store := UI.button("Shop", "ghost", 15, Vector2(66, 56))
	store.pressed.connect(func(): open_store.emit())
	bar.add_child(store)
	return bar


func _bet_bar() -> VBoxContainer:
	var v := VBoxContainer.new()
	v.add_theme_constant_override("separation", 10)

	var row := HBoxContainer.new()
	row.add_theme_constant_override("separation", 10)
	row.alignment = BoxContainer.ALIGNMENT_CENTER
	v.add_child(row)

	var minus := UI.button("–", "ghost", 26, Vector2(56, 56))
	minus.pressed.connect(func(): _change_bet(-1))
	row.add_child(minus)

	var bet_box := UI.panel(Color(0, 0, 0, 0.3), 16, Color(1, 1, 1, 0.1), 1, 10)
	bet_box.custom_minimum_size = Vector2(170, 0)
	bet_label = UI.label("", 20, UI.TEXT, HORIZONTAL_ALIGNMENT_CENTER)
	bet_box.add_child(bet_label)
	row.add_child(bet_box)

	var plus := UI.button("+", "ghost", 26, Vector2(56, 56))
	plus.pressed.connect(func(): _change_bet(1))
	row.add_child(plus)

	spin_button = UI.button("SPIN", "gold", 30, Vector2(170, 76))
	spin_button.pressed.connect(_on_spin_pressed)
	row.add_child(spin_button)

	_update_bet_label()
	return v


func _lines() -> int:
	return int(game_info.get("lines", 1))


func _bet_per_line() -> int:
	return Session.BET_LEVELS[Session.bet_level_idx]


func _total_bet() -> int:
	return _bet_per_line() * _lines()


func _change_bet(dir: int) -> void:
	Session.bet_level_idx = clampi(Session.bet_level_idx + dir, 0, Session.BET_LEVELS.size() - 1)
	Session.save()
	_update_bet_label()
	Sfx.play("tick")


func _update_bet_label() -> void:
	bet_label.text = "Bet: %s  (%d lines)" % [UI.fmt(_total_bet()), _lines()]


func _on_spin_pressed() -> void:
	if reels.is_spinning():
		return
	if _total_bet() > Session.total_balance():
		status_label.text = "Not enough coins for this bet."
		Sfx.play("error")
		Session.vibrate(40)
		low_balance_during_play.emit()
		return

	spin_button.disabled = true
	status_label.text = ""
	win_label.text = "Spinning..."
	Session.preview_deduct(_total_bet())

	var key := Session.api.new_idempotency_key()
	var res: Dictionary = await Session.api.spin(game_id, _bet_per_line(), _lines(), key)

	if not res.ok or not (res.data is Dictionary):
		spin_button.disabled = false
		win_label.text = "Spin failed."
		status_label.text = (res.data.error if res.data is Dictionary and res.data.has("error") else res.raw)
		Sfx.play("error")
		var bal: Dictionary = await Session.api.get_balance()
		if bal.ok and bal.data is Dictionary:
			Session.apply_balance(int(bal.data.get("purchased", 0)), int(bal.data.get("bonus", 0)))
		return

	_pending_result = res.data
	await reels.spin_to(res.data.get("grid", []))


var _pending_result: Dictionary


func _on_spin_finished() -> void:
	var data := _pending_result
	spin_button.disabled = false
	Session.apply_balance(int(data.get("purchasedBalance", 0)), int(data.get("bonusBalance", 0)))

	var total_win := int(data.get("totalWin", 0))
	if total_win > 0:
		reels.highlight_wins(data.get("lineWins", []), game_info.get("paylines", []))
		win_label.text = "WIN  +%s coins" % UI.fmt(total_win)
		win_label.add_theme_color_override("font_color", UI.GOLD)
		var big := total_win >= _total_bet() * 10
		Sfx.play("win_big" if big else "win_small")
		Session.vibrate(60 if big else 25)
		_pulse(win_label)
	else:
		win_label.text = "No win this time."
		win_label.add_theme_color_override("font_color", UI.MUTED)

	if Session.total_balance() <= 0:
		status_label.text = "Out of coins — visit the store to keep playing."
		low_balance_during_play.emit()


func _pulse(n: Control) -> void:
	n.scale = Vector2(0.85, 0.85)
	n.pivot_offset = n.size / 2.0
	var tw := n.create_tween()
	tw.tween_property(n, "scale", Vector2(1.08, 1.08), 0.12).set_trans(Tween.TRANS_BACK).set_ease(Tween.EASE_OUT)
	tw.tween_property(n, "scale", Vector2.ONE, 0.10)
