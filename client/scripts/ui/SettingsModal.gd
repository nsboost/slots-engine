class_name SettingsModal
extends RefCounted

static func open(host: Control) -> void:
	var m := Modal.open(host, "Settings", 600)

	m.body.add_child(_toggle_row("Sound effects", Session.sound_on, func(v): Session.set_sound(v)))
	m.body.add_child(_toggle_row("Haptics", Session.haptics_on, func(v): Session.set_haptics(v)))

	m.body.add_child(UI.spacer(6))
	m.body.add_child(UI.label("Player ID", 14, UI.MUTED))
	var id_row := UI.panel(Color(0, 0, 0, 0.25), 12, Color(1, 1, 1, 0.1), 1, 10)
	id_row.add_child(UI.label(Session.player_id, 16, UI.TEXT))
	m.body.add_child(id_row)

	if not OS.has_feature("web"):
		m.body.add_child(UI.label("Server address", 14, UI.MUTED))
		var edit := LineEdit.new()
		edit.text = Session.base_url
		edit.custom_minimum_size = Vector2(0, 52)
		m.body.add_child(edit)
		var apply := UI.button("Update server", "ghost", 18, Vector2(0, 52))
		apply.pressed.connect(func():
			Session.set_server_url(edit.text)
			UI.toast(m, "Server updated. Reopen to reconnect.", UI.GREEN)
		)
		m.body.add_child(apply)

	m.body.add_child(UI.spacer(6))
	var reset := UI.button("Reset guest account", "red", 18, Vector2(0, 52))
	reset.pressed.connect(func(): _confirm_reset(m))
	m.body.add_child(reset)

	m.body.add_child(UI.label("v%s" % Session.APP_VERSION, 13, UI.MUTED, HORIZONTAL_ALIGNMENT_CENTER))
	m.add_close_button()


static func _toggle_row(text: String, value: bool, on_change: Callable) -> HBoxContainer:
	var row := HBoxContainer.new()
	var lbl := UI.label(text, 20, UI.TEXT)
	lbl.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	row.add_child(lbl)
	var cb := CheckButton.new()
	cb.button_pressed = value
	cb.toggled.connect(on_change)
	row.add_child(cb)
	return row


static func _confirm_reset(modal: Modal) -> void:
	for c in modal.body.get_children():
		c.queue_free()
	modal.body.add_child(UI.label("This creates a brand new guest account\nand forgets your current balance.", 18, UI.MUTED, HORIZONTAL_ALIGNMENT_CENTER))
	var row := HBoxContainer.new()
	row.alignment = BoxContainer.ALIGNMENT_CENTER
	row.add_theme_constant_override("separation", 12)
	modal.body.add_child(row)
	var cancel := UI.button("Cancel", "ghost", 20, Vector2(140, 60))
	cancel.pressed.connect(modal.close)
	row.add_child(cancel)
	var confirm := UI.button("Reset", "red", 20, Vector2(140, 60))
	confirm.pressed.connect(func():
		Session.reset_account()
		var tree := modal.get_tree()
		modal.close()
		tree.reload_current_scene()
	)
	row.add_child(confirm)
