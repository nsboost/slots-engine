# LobbyScreen: pick a game. Pulls the catalog from Session (already loaded
# by SplashScreen) and builds one themed card per game.
class_name LobbyScreen
extends Control

signal game_selected(game_id: String)
signal open_store
signal open_daily
signal open_settings

var backdrop: Backdrop


func _ready() -> void:
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	backdrop = Backdrop.new()
	add_child(backdrop)

	var margin := MarginContainer.new()
	margin.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	for side in ["margin_left", "margin_right", "margin_top", "margin_bottom"]:
		margin.add_theme_constant_override(side, 20)
	add_child(margin)

	var root := VBoxContainer.new()
	root.add_theme_constant_override("separation", 16)
	margin.add_child(root)

	root.add_child(_top_bar())
	root.add_child(UI.label("Choose a game", 24, UI.TEXT, HORIZONTAL_ALIGNMENT_CENTER))

	var scroll := ScrollContainer.new()
	scroll.size_flags_vertical = Control.SIZE_EXPAND_FILL
	root.add_child(scroll)
	var list := VBoxContainer.new()
	list.add_theme_constant_override("separation", 16)
	list.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	scroll.add_child(list)

	for g in Session.games:
		list.add_child(_game_card(g))

	Session.balance_changed.connect(func(): pass)  # pill already listens itself


func _top_bar() -> HBoxContainer:
	var bar := HBoxContainer.new()
	bar.add_theme_constant_override("separation", 10)

	var pill := BalancePill.new()
	bar.add_child(pill)
	bar.add_child(UI.spacer(0, 8))

	var daily := UI.button("Gifts", "ghost", 16, Vector2(66, 60))
	daily.pressed.connect(func(): open_daily.emit())
	bar.add_child(daily)

	var store := UI.button("Shop", "ghost", 16, Vector2(66, 60))
	store.pressed.connect(func(): open_store.emit())
	bar.add_child(store)

	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	bar.add_child(spacer)

	var settings := UI.button("Menu", "ghost", 15, Vector2(66, 60))
	settings.pressed.connect(func(): open_settings.emit())
	bar.add_child(settings)
	return bar


func _game_card(g: Dictionary) -> PanelContainer:
	var id := str(g.get("id", ""))
	var theme: Dictionary = GameThemes.for_game(id)

	var card := UI.panel(theme.bg_top.lightened(0.0).darkened(0.35), 22, theme.frame, 2, 18)
	var v := VBoxContainer.new()
	v.add_theme_constant_override("separation", 10)
	card.add_child(v)

	v.add_child(UI.label(str(g.get("name", id)), 28, theme.accent, HORIZONTAL_ALIGNMENT_LEFT, 2))
	v.add_child(UI.label(str(theme.tagline), 16, UI.MUTED))

	var preview := HBoxContainer.new()
	preview.add_theme_constant_override("separation", 8)
	v.add_child(preview)
	for code in theme.preview:
		var sv := SymbolView.new()
		sv.custom_minimum_size = Vector2(54, 54)
		sv.set_code(code)
		preview.add_child(sv)
	var lines_lbl := UI.label("%d paylines" % int(g.get("lines", 0)), 16, UI.MUTED)
	lines_lbl.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	lines_lbl.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	preview.add_child(lines_lbl)

	var play := UI.button("PLAY", "gold", 24, Vector2(0, 64))
	play.pressed.connect(func(): game_selected.emit(id))
	v.add_child(play)

	return card
