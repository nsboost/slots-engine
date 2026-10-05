# Sfx: procedurally generated sound effects. No audio files to ship or
# import — each effect is synthesized into an AudioStreamWAV at startup
# (a few hundred ms of PCM each), which also keeps the repo asset-free.
# Swap in recorded audio later by replacing entries in _streams.
extends Node

var enabled: bool = true

const RATE := 22050
var _players: Array[AudioStreamPlayer] = []
var _streams: Dictionary = {}


func _ready() -> void:
	for i in 6:
		var p := AudioStreamPlayer.new()
		add_child(p)
		_players.append(p)

	_streams["click"] = _make([[1040.0, 0.035]], 0.30)
	_streams["spin_start"] = _make([[330.0, 0.05], [392.0, 0.05], [494.0, 0.05], [587.0, 0.07]], 0.28)
	_streams["reel_stop"] = _make([[150.0, 0.09]], 0.55, 0.0)
	_streams["win_small"] = _make([[523.0, 0.09], [659.0, 0.09], [784.0, 0.16]], 0.38)
	_streams["win_big"] = _make([[523.0, 0.1], [659.0, 0.1], [784.0, 0.1], [1047.0, 0.14], [784.0, 0.08], [1047.0, 0.08], [1319.0, 0.3]], 0.42)
	_streams["coin"] = _make([[1319.0, 0.06], [1760.0, 0.18]], 0.34)
	_streams["error"] = _make([[220.0, 0.12], [165.0, 0.18]], 0.35, 0.25)
	_streams["tick"] = _make([[700.0, 0.012]], 0.18)


## notes: [[freq_hz, seconds], ...] (freq 0 = rest). `noise` mixes in
## a fraction of white noise (for thuds / error buzz).
func _make(notes: Array, vol: float, noise: float = 0.0) -> AudioStreamWAV:
	var data := PackedByteArray()
	for n in notes:
		var f: float = n[0]
		var d: float = n[1]
		var count := int(RATE * d)
		var fade_in := RATE * 0.004
		var fade_out := RATE * 0.04
		for i in count:
			var t := float(i) / RATE
			var env := minf(1.0, float(i) / fade_in) * minf(1.0, float(count - i) / fade_out)
			var v := 0.0
			if f > 0.0:
				v = sin(TAU * f * t) * 0.8 + sin(TAU * f * 2.0 * t) * 0.2
			if noise > 0.0:
				v = v * (1.0 - noise) + randf_range(-1.0, 1.0) * noise
			var s := int(clampf(v * env * vol, -1.0, 1.0) * 32767.0)
			data.append(s & 0xFF)
			data.append((s >> 8) & 0xFF)
	var w := AudioStreamWAV.new()
	w.format = AudioStreamWAV.FORMAT_16_BITS
	w.mix_rate = RATE
	w.stereo = false
	w.data = data
	return w


func play(sound: String) -> void:
	if not enabled or not _streams.has(sound):
		return
	var chosen: AudioStreamPlayer = null
	for p in _players:
		if not p.playing:
			chosen = p
			break
	if chosen == null:
		chosen = _players[0]
	chosen.stream = _streams[sound]
	chosen.play()
