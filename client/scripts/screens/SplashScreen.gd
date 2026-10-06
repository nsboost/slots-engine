# SplashScreen: connects to the server, creates/restores a guest account,
# and loads the game catalog before handing off to the Lobby. Shows a
# clear retry path if the server can't be reached — this is the screen
# most likely to surface "demo link is asleep" style issues, so it needs
# to fail loudly and helpfully, not just spin forever.
class_name SplashScreen
extends Control

signal ready_to_play

var _status: Label
var _retry: Button
var _spinner: Control


func _ready() -> void:
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	var center := CenterContainer.new()
	center.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(center)

	var v := VBoxContainer.new()
	v.add_theme_constant_override("separation", 22)
	v.alignment = BoxContainer.ALIGNMENT_CENTER
	center.add_child(v)

	var title := UI.label("Fortune Reels", 46, UI.GOLD, HORIZONTAL_ALIGNMENT_CENTER, 4)
	v.add_child(title)
	v.add_child(UI.label("social casino demo", 18, UI.MUTED, HORIZONTAL_ALIGNMENT_CENTER))
	v.add_child(UI.spacer(20))

	_spinner = _make_spinner()
	v.add_child(_spinner)

	_status = UI.label("Connecting...", 18, UI.TEXT, HORIZONTAL_ALIGNMENT_CENTER)
	_status.custom_minimum_size = Vector2(440, 0)
	_status.autowrap_mode = TextServer.AUTOWRAP_WORD
	v.add_child(_status)

	_retry = UI.button("Retry", "gold", 22, Vector2(200, 64))
	_retry.visible = false
	_retry.pressed.connect(_start)
	v.add_child(_retry)

	_start()


func _make_spinner() -> Control:
	var c := Control.new()
	c.custom_minimum_size = Vector2(56, 56)
	var dot := ColorRect.new()
	dot.color = UI.GOLD
	dot.size = Vector2(56, 56)
	dot.set_script(null)
	c.add_child(_SpinnerDraw.new())
	return c


func _start() -> void:
	_retry.visible = false
	_status.text = "Connecting to server..."
	_set_spinning(true)

	var acc: Dictionary = await Session.ensure_account()
	if not acc.ok:
		_fail(str(acc.get("error", "Could not connect.")))
		return

	_status.text = "Loading games..."
	var cat: Dictionary = await Session.load_catalog()
	if not cat.ok:
		_fail(str(cat.get("error", "Could not load games.")))
		return

	var is_mock: bool = Session.api.is_mock()
	if is_mock:
		_status.text = "Running in demo mode (no server — full game works offline)"
		await get_tree().create_timer(1.2).timeout
	elif bool(acc.get("new_account", false)):
		_status.text = "Welcome! +%s bonus coins credited." % UI.fmt(int(acc.get("welcome", 0)))
		await get_tree().create_timer(0.9).timeout
	_set_spinning(false)
	ready_to_play.emit()


func _fail(msg: String) -> void:
	_set_spinning(false)
	_status.text = msg
	_retry.visible = true
	Sfx.play("error")


func _set_spinning(v: bool) -> void:
	_spinner.visible = v


class _SpinnerDraw:
	extends Control
	var _t := 0.0
	func _ready() -> void:
		custom_minimum_size = Vector2(56, 56)
	func _process(delta: float) -> void:
		_t += delta * 3.2
		queue_redraw()
	func _draw() -> void:
		var c := size / 2.0
		var r := 24.0
		draw_arc(c, r, _t, _t + TAU * 0.68, 24, UI.GOLD, 6.0, true)
