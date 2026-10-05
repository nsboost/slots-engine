class_name StoreModal
extends RefCounted

static func open(host: Control) -> void:
	var m := Modal.open(host, "Coin Store", 620)
	m.body.add_child(UI.label("Demo mode — no real payment is taken.", 16, UI.MUTED, HORIZONTAL_ALIGNMENT_CENTER))

	if Session.packages.is_empty():
		m.body.add_child(UI.label("Could not load the store.", 20, UI.RED, HORIZONTAL_ALIGNMENT_CENTER))
	for pkg in Session.packages:
		m.body.add_child(_row(m, pkg))

	m.add_close_button()


static func _row(modal: Modal, pkg: Dictionary) -> PanelContainer:
	var p := UI.panel(Color(1, 1, 1, 0.05), 18, Color(1, 1, 1, 0.10), 1, 14)
	var h := HBoxContainer.new()
	h.add_theme_constant_override("separation", 12)
	p.add_child(h)

	var info := VBoxContainer.new()
	info.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	h.add_child(info)
	info.add_child(UI.label(str(pkg.get("name", "")), 22, UI.TEXT))
	var coins := int(pkg.get("coins", 0))
	var bonus := int(pkg.get("bonus", 0))
	var detail := "%s coins" % UI.fmt(coins)
	if bonus > 0:
		detail += " + %s bonus" % UI.fmt(bonus)
	info.add_child(UI.label(detail, 16, UI.MUTED))

	var buy := UI.button(str(pkg.get("priceLabel", "")), "green", 22, Vector2(130, 60))
	buy.pressed.connect(func(): _buy(modal, buy, str(pkg.get("id", ""))))
	h.add_child(buy)
	return p


static func _buy(modal: Modal, button: Button, package_id: String) -> void:
	button.disabled = true
	var res: Dictionary = await Session.api.purchase(package_id)
	if res.ok and res.data is Dictionary:
		Session.apply_balance(int(res.data.get("purchased", 0)), int(res.data.get("bonus", 0)))
		Sfx.play("coin")
		UI.toast(modal, "Purchase complete!", UI.GREEN)
		modal.close()
	else:
		button.disabled = false
		Sfx.play("error")
		UI.toast(modal, "Purchase failed. Try again.", UI.RED)
