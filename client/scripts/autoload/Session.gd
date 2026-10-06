# Session: the app's global state — account, balances, catalog, settings.
# Screens read from here and listen to signals; they never talk to the
# network layer for account/balance bookkeeping themselves.
extends Node

signal balance_changed
signal settings_changed

const SETTINGS_PATH := "user://settings.cfg"
const APP_VERSION := "0.3.0"

var api: ApiClient

# Account
var base_url: String = ""
var player_id: String = ""
var token: String = ""

# Balances. `purchased`/`bonus` are authoritative (from the server);
# `display_total` is what the UI shows (it dips by the bet at spin start and
# is reconciled with the server result when the spin resolves).
var purchased: int = 0
var bonus: int = 0
var display_total: int = 0

# Catalog (from the server)
var games: Array = []
var packages: Array = []

# Settings
var sound_on: bool = true
var haptics_on: bool = true
var bet_level_idx: int = 1

const BET_LEVELS: Array[int] = [1, 2, 5, 10, 20, 50]


func _ready() -> void:
	api = ApiClient.new()
	add_child(api)
	_load()
	_apply_settings()


func default_server_url() -> String:
	if OS.has_feature("web"):
		# Check for an injected override first (GitHub Pages pointing at a hosted server);
		# fall back to same-origin (when the Go server also serves the HTML).
		var override = str(JavaScriptBridge.eval("window.SLOTS_SERVER_URL || ''"))
		if override != "" and override != "null" and override != "$SERVER_URL":
			return override
		return str(JavaScriptBridge.eval("window.location.origin"))
	if OS.get_name() == "Android":
		return "http://10.0.2.2:8080"  # emulator -> host loopback; real devices need the LAN IP
	return "http://127.0.0.1:8080"


func _load() -> void:
	var cf := ConfigFile.new()
	var err := cf.load(SETTINGS_PATH)
	base_url = default_server_url()
	if err == OK:
		if not OS.has_feature("web"):
			base_url = str(cf.get_value("server", "url", base_url))
		player_id = str(cf.get_value("account", "player_id", ""))
		token = str(cf.get_value("account", "token", ""))
		sound_on = bool(cf.get_value("settings", "sound", true))
		haptics_on = bool(cf.get_value("settings", "haptics", true))
		bet_level_idx = clampi(int(cf.get_value("settings", "bet_idx", 1)), 0, BET_LEVELS.size() - 1)


func save() -> void:
	var cf := ConfigFile.new()
	cf.set_value("server", "url", base_url)
	cf.set_value("account", "player_id", player_id)
	cf.set_value("account", "token", token)
	cf.set_value("settings", "sound", sound_on)
	cf.set_value("settings", "haptics", haptics_on)
	cf.set_value("settings", "bet_idx", bet_level_idx)
	cf.save(SETTINGS_PATH)


func _apply_settings() -> void:
	Sfx.enabled = sound_on
	api.base_url = base_url
	api.player_id = player_id
	api.token = token


func set_sound(v: bool) -> void:
	sound_on = v
	_apply_settings()
	save()
	settings_changed.emit()


func set_haptics(v: bool) -> void:
	haptics_on = v
	save()
	settings_changed.emit()


func set_server_url(url: String) -> void:
	base_url = url.strip_edges().rstrip("/")
	_apply_settings()
	save()


func reset_account() -> void:
	player_id = ""
	token = ""
	purchased = 0
	bonus = 0
	display_total = 0
	_apply_settings()
	save()
	balance_changed.emit()


func vibrate(ms: int) -> void:
	if haptics_on and (OS.get_name() == "Android" or OS.get_name() == "iOS"):
		Input.vibrate_handheld(ms)


# --- balances ----------------------------------------------------------

func total_balance() -> int:
	return purchased + bonus


func apply_balance(p: int, b: int) -> void:
	purchased = p
	bonus = b
	display_total = p + b
	balance_changed.emit()


## Shows the bet leaving the balance the moment a spin starts, before the
## server confirms (the real values arrive with the spin result).
func preview_deduct(amount: int) -> void:
	display_total = max(0, total_balance() - amount)
	balance_changed.emit()


# --- boot flow ---------------------------------------------------------

## Makes sure there is a working account. Returns
## {ok, error, new_account, welcome}.
func ensure_account() -> Dictionary:
	_apply_settings()
	if token != "" and player_id != "":
		var r: Dictionary = await api.get_balance()
		if r.ok and r.data is Dictionary:
			apply_balance(int(r.data.get("purchased", 0)), int(r.data.get("bonus", 0)))
			return {"ok": true, "new_account": false, "welcome": 0}
		if r.code == 401 or r.code == 403:
			reset_account()  # token no longer valid -> new guest
		elif r.code == 0 or not r.ok:
			# Server unreachable — api will use mock mode, just proceed
			apply_balance(0, 1000)  # mock welcome bonus
			return {"ok": true, "new_account": true, "welcome": 1000}
		else:
			return {"ok": false, "error": _friendly_error(r)}

	var g: Dictionary = await api.guest()
	if not g.ok or not (g.data is Dictionary):
		return {"ok": false, "error": _friendly_error(g)}
	player_id = str(g.data.get("playerId", ""))
	token = str(g.data.get("token", ""))
	_apply_settings()
	save()
	var welcome: int = int(g.data.get("welcomeBonus", 0))
	apply_balance(0, welcome)
	return {"ok": true, "new_account": true, "welcome": welcome}


func load_catalog() -> Dictionary:
	var g: Dictionary = await api.games()
	if not g.ok or not (g.data is Dictionary):
		return {"ok": false, "error": _friendly_error(g)}
	games = g.data.get("games", [])
	var p: Dictionary = await api.packages()
	if p.ok and p.data is Dictionary:
		packages = p.data.get("packages", [])
	return {"ok": true}


func game_by_id(id: String) -> Dictionary:
	for g in games:
		if g.get("id", "") == id:
			return g
	return {}


func _friendly_error(r: Dictionary) -> String:
	if r.code == 0:
		return "Can't reach the server.\n%s" % r.raw
	var msg: String = r.raw
	if r.data is Dictionary and r.data.has("error"):
		msg = str(r.data.error)
	return "Server said (%d): %s" % [r.code, msg]
