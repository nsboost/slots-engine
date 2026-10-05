# Main: top-level router. Splash -> Lobby <-> Spin, with Store/Daily/
# Settings as modals over whichever screen is active. One small state
# machine instead of Godot's scene-switcher, since every screen here is
# built in code (see each screen's own file) and swapping a Control child
# is all "changing scenes" means at this scale.
extends Control

var _current: Control


func _ready() -> void:
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	_show_splash()


func restart() -> void:
	if _current:
		_current.queue_free()
	_show_splash()


func _clear() -> void:
	if _current:
		_current.queue_free()
		_current = null


func _show_splash() -> void:
	_clear()
	var s := SplashScreen.new()
	add_child(s)
	_current = s
	s.ready_to_play.connect(_show_lobby)


func _show_lobby() -> void:
	_clear()
	var l := LobbyScreen.new()
	add_child(l)
	_current = l
	l.game_selected.connect(_show_spin)
	l.open_store.connect(func(): StoreModal.open(self))
	l.open_daily.connect(func(): DailyBonusModal.open(self))
	l.open_settings.connect(func(): SettingsModal.open(self))


func _show_spin(game_id: String) -> void:
	_clear()
	var sc := SpinScreen.new()
	add_child(sc)
	sc.setup(game_id)
	_current = sc
	sc.back_pressed.connect(_show_lobby)
	sc.open_store.connect(func(): StoreModal.open(self))
	sc.low_balance_during_play.connect(func(): StoreModal.open(self))
