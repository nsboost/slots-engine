# GameThemes: per-game look. Adding a game = one entry here (plus the
# server-side config in internal/engine/games.go).
class_name GameThemes
extends RefCounted

const THEMES := {
	"fortune-reels": {
		"bg_top": Color("#2c1068"), "bg_bottom": Color("#0c0524"),
		"accent": Color("#ffcf4a"), "frame": Color("#6a35c9"),
		"tagline": "Classic reels. Steady wins.",
		"preview": ["SA", "WILD", "B2"],
	},
	"gold-rush": {
		"bg_top": Color("#5a2a0a"), "bg_bottom": Color("#190b02"),
		"accent": Color("#ffb347"), "frame": Color("#b8651b"),
		"tagline": "15 lines. Big nuggets.",
		"preview": ["B1", "WILD", "SCATTER"],
	},
}


static func for_game(id: String) -> Dictionary:
	return THEMES.get(id, THEMES["fortune-reels"])
