class_name DailyBonusModal
extends RefCounted

static func open(host: Control) -> void:
	var m := Modal.open(host, "Daily Bonus", 600)
	var loading := UI.label("Loading...", 18, UI.MUTED, HORIZONTAL_ALIGNMENT_CENTER)
	m.body.add_child(loading)

	var res: Dictionary = await Session.api.daily_status()
	loading.queue_free()
	if not res.ok or not (res.data is Dictionary):
		m.body.add_child(UI.label("Could not load the daily bonus.", 18, UI.RED, HORIZONTAL_ALIGNMENT_CENTER))
		m.add_close_button()
		return

	var status: Dictionary = res.data
	var rewards: Array = status.get("rewards", [])
	var day := int(status.get("day", 1))

	var ladder := HBoxContainer.new()
	ladder.add_theme_constant_override("separation", 6)
	ladder.alignment = BoxContainer.ALIGNMENT_CENTER
	m.body.add_child(ladder)
	for i in rewards.size():
		ladder.add_child(_step(i + 1, int(rewards[i]), i + 1 == day, i + 1 < day))

	var claimed: bool = status.get("claimed", false)
	var claim_btn := UI.button(
		("Already claimed today" if claimed else "Claim %s coins" % UI.fmt(int(status.get("reward", 0)))),
		("ghost" if claimed else "gold"), 24
	)
	claim_btn.disabled = claimed
	m.body.add_child(claim_btn)
	claim_btn.pressed.connect(func(): _claim(m, claim_btn))

	m.add_close_button("Close", "purple")


static func _step(day: int, reward: int, is_today: bool, is_done: bool) -> PanelContainer:
	var bg := Color(1, 1, 1, 0.06)
	var border := Color(1, 1, 1, 0.12)
	if is_today:
		bg = Color(1.0, 0.85, 0.35, 0.22); border = UI.GOLD
	elif is_done:
		bg = Color(0.2, 0.8, 0.5, 0.16); border = UI.GREEN
	var p := UI.panel(bg, 14, border, 2, 8)
	p.custom_minimum_size = Vector2(66, 72)
	var v := VBoxContainer.new()
	v.alignment = BoxContainer.ALIGNMENT_CENTER
	p.add_child(v)
	v.add_child(UI.label("D%d" % day, 13, UI.MUTED, HORIZONTAL_ALIGNMENT_CENTER))
	v.add_child(UI.label(UI.fmt(reward), 15, UI.TEXT, HORIZONTAL_ALIGNMENT_CENTER))
	if is_done:
		v.add_child(UI.label("✓", 14, UI.GREEN, HORIZONTAL_ALIGNMENT_CENTER))
	return p


static func _claim(modal: Modal, button: Button) -> void:
	button.disabled = true
	var res: Dictionary = await Session.api.daily_claim()
	if res.ok and res.data is Dictionary:
		var bal: Dictionary = await Session.api.get_balance()
		if bal.ok and bal.data is Dictionary:
			Session.apply_balance(int(bal.data.get("purchased", 0)), int(bal.data.get("bonus", 0)))
		Sfx.play("coin")
		Session.vibrate(30)
		UI.toast(modal, "+%s coins!" % UI.fmt(int(res.data.get("reward", 0))), UI.GREEN)
		modal.close()
	else:
		button.disabled = false
		UI.toast(modal, "Could not claim right now.", UI.RED)
