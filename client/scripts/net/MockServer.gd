# MockServer: a fully client-side slot engine for the standalone web demo.
# Mirrors the exact JSON shape of the real Go backend so ApiClient + Session
# work identically whether they're talking to a real server or this mock.
# Activated automatically when the real server isn't reachable.
class_name MockServer
extends RefCounted

# Simple PRNG (xorshift32) — deterministic, fast, good enough for a demo.
var _seed: int = 12345678

func _rand() -> float:
	_seed ^= _seed << 13
	_seed &= 0xFFFFFFFF
	_seed ^= _seed >> 17
	_seed &= 0xFFFFFFFF
	_seed ^= _seed << 5
	_seed &= 0xFFFFFFFF
	return float(_seed & 0xFFFFFF) / float(0xFFFFFF)

func _randi_range(a: int, b: int) -> int:
	return a + int(_rand() * float(b - a + 1))

# ── Symbol tables ────────────────────────────────────────────────────
const STRIP_FORTUNE := ["S10","SJ","SQ","SK","SA","S10","SJ","SQ","SK","SA",
	"S10","SJ","SQ","SK","S10","SJ","SQ","SK","SA",
	"B1","B1","B2","WILD","SCATTER","S10","SJ","SQ"]

const STRIP_GOLD := ["S10","SJ","SQ","SK","SA","S10","SJ","SQ","SK","SA",
	"S10","SJ","SQ","SK","SA","S10","SJ","SQ","SK","SA",
	"B1","B1","B1","B2","B2","WILD","SCATTER","SCATTER"]

const PAYLINES_FORTUNE := [
	[1,1,1,1,1],[0,0,0,0,0],[2,2,2,2,2],
	[0,1,2,1,0],[2,1,0,1,2],[1,0,0,0,1],
	[1,2,2,2,1],[0,0,1,2,2],[2,2,1,0,0]
]

const PAYLINES_GOLD := [
	[1,1,1,1,1],[0,0,0,0,0],[2,2,2,2,2],
	[0,1,2,1,0],[2,1,0,1,2],[0,0,1,2,2],[2,2,1,0,0],
	[1,0,0,0,1],[1,2,2,2,1],[0,1,1,1,0],[2,1,1,1,2],
	[1,0,1,2,1],[1,2,1,0,1],[0,1,0,1,0],[2,1,2,1,2]
]

const PAYTABLE := {
	"S10":{"3":4,"4":10,"5":30},
	"SJ": {"3":5,"4":15,"5":45},
	"SQ": {"3":7,"4":22,"5":70},
	"SK": {"3":9,"4":36,"5":135},
	"SA": {"3":14,"4":60,"5":270},
	"B1": {"3":18,"4":88,"5":440},
	"B2": {"3":44,"4":220,"5":1750},
	"WILD":{"3":53,"4":265,"5":2650},
}
const SCATTER_PAYS := {"3":4,"4":17,"5":85}
const WILD := "WILD"
const SCATTER := "SCATTER"

# ── State ────────────────────────────────────────────────────────────
var _player_id: String = ""
var _purchased: int = 0
var _bonus: int = 0
var _daily_claimed_date: String = ""
var _daily_streak: int = 0
var _applied_txids: Dictionary = {}

const DAILY_REWARDS := [500, 750, 1000, 1500, 2000, 3000, 5000]

func _today_str() -> String:
	return Time.get_date_string_from_system()

# ── Public API ───────────────────────────────────────────────────────
func health() -> Dictionary:
	return {"ok":true,"code":200,"data":{"status":"ok","gameVersion":"0.1.0-demo (mock)"},"raw":""}

func guest() -> Dictionary:
	_player_id = "mock-" + str(int(Time.get_ticks_usec() % 1000000)).pad_zeros(6)
	_bonus = 1000
	return {"ok":true,"code":201,"data":{
		"playerId":_player_id,"token":"mock-token","welcomeBonus":1000},"raw":""}

func get_balance() -> Dictionary:
	return {"ok":true,"code":200,"data":{"purchased":_purchased,"bonus":_bonus},"raw":""}

func games() -> Dictionary:
	return {"ok":true,"code":200,"data":{"games":[_fortune_info(), _gold_info()]},"raw":""}

func packages() -> Dictionary:
	return {"ok":true,"code":200,"data":{"demo":true,"packages":[
		{"id":"starter","name":"Starter Stash","coins":1000,"bonus":0,"priceLabel":"$0.99"},
		{"id":"popular","name":"Lucky Pile","coins":5500,"bonus":500,"priceLabel":"$4.99"},
		{"id":"chest","name":"Treasure Chest","coins":12000,"bonus":2000,"priceLabel":"$9.99"},
		{"id":"vault","name":"Mega Vault","coins":32000,"bonus":8000,"priceLabel":"$19.99"},
	]},"raw":""}

func purchase(package_id: String) -> Dictionary:
	var pkgs := {"starter":[1000,0],"popular":[5500,500],"chest":[12000,2000],"vault":[32000,8000]}
	if not pkgs.has(package_id):
		return {"ok":false,"code":400,"data":{"error":"unknown package"},"raw":""}
	_purchased += pkgs[package_id][0]; _bonus += pkgs[package_id][1]
	return {"ok":true,"code":200,"data":{"purchased":_purchased,"bonus":_bonus},"raw":""}

func daily_status() -> Dictionary:
	var today := _today_str()
	var claimed := (_daily_claimed_date == today)
	var day := _daily_streak if claimed else (_daily_streak % 7) + 1
	if not claimed: day = (_daily_streak % 7) + 1
	return {"ok":true,"code":200,"data":{
		"day":day,"reward":DAILY_REWARDS[day-1],"claimed":claimed,
		"rewards":DAILY_REWARDS,"nextClaimAt":today+"T24:00:00Z"},"raw":""}

func daily_claim() -> Dictionary:
	var today := _today_str()
	if _daily_claimed_date == today:
		var day := _daily_streak
		return {"ok":true,"code":200,"data":{"day":day,"reward":DAILY_REWARDS[day-1],"claimed":true,"replayed":true,"rewards":DAILY_REWARDS,"nextClaimAt":today+"T24:00:00Z"},"raw":""}
	_daily_streak = (_daily_streak % 7) + 1
	_daily_claimed_date = today
	var reward: int = int(DAILY_REWARDS[_daily_streak - 1])
	_bonus += reward
	return {"ok":true,"code":200,"data":{"day":_daily_streak,"reward":reward,"claimed":true,"rewards":DAILY_REWARDS,"nextClaimAt":today+"T24:00:00Z"},"raw":""}

func spin(game_id: String, bet_per_line: int, lines: int, idempotency_key: String) -> Dictionary:
	if _applied_txids.has(idempotency_key):
		var cached = _applied_txids[idempotency_key]
		cached["replayed"] = true
		cached["purchasedBalance"] = _purchased
		cached["bonusBalance"] = _bonus
		return {"ok":true,"code":200,"data":cached,"raw":""}
	var total_bet := bet_per_line * lines
	if total_bet > _purchased + _bonus:
		return {"ok":false,"code":402,"data":{"error":"insufficient balance"},"raw":""}
	var strip := STRIP_FORTUNE if game_id == "fortune-reels" else STRIP_GOLD
	var paylines := PAYLINES_FORTUNE if game_id == "fortune-reels" else PAYLINES_GOLD
	var grid := []
	for col in 5:
		var stop := _randi_range(0, len(strip) - 1)
		var col_syms := []
		for r in 3: col_syms.append(strip[(stop + r) % len(strip)])
		grid.append(col_syms)
	var line_wins := []
	var total_win := 0
	for i in range(mini(lines, len(paylines))):
		var win := _eval_line(grid, paylines[i], bet_per_line)
		if win["win"] > 0:
			win["lineIndex"] = i; line_wins.append(win); total_win += win["win"]
	var scatter_count := 0
	for col in grid:
		for sym in col:
			if sym == SCATTER: scatter_count += 1
	var scatter_win := 0
	var sk := str(scatter_count)
	if SCATTER_PAYS.has(sk): scatter_win = total_bet * SCATTER_PAYS[sk]; total_win += scatter_win
	# Deduct from purchased first, then bonus
	var from_purchased := mini(_purchased, total_bet)
	var from_bonus := total_bet - from_purchased
	_purchased -= from_purchased; _bonus -= from_bonus
	_purchased += total_win
	var result := {"gameId":game_id,"gameVersion":"0.1.0-demo (mock)","grid":grid,
		"lineWins":line_wins,"scatterWin":scatter_win,"totalWin":total_win,"totalBet":total_bet,
		"purchasedBalance":_purchased,"bonusBalance":_bonus,"replayed":false}
	_applied_txids[idempotency_key] = result.duplicate()
	return {"ok":true,"code":200,"data":result,"raw":""}

func _eval_line(grid: Array, line: Array, bet: int) -> Dictionary:
	var target := str(grid[0][line[0]])
	if target == WILD:
		for col in range(1, 5):
			var s := str(grid[col][line[col]])
			if s != WILD and s != SCATTER: target = s; break
	if target == SCATTER: return {"win":0}
	if not PAYTABLE.has(target): return {"win":0}
	var count := 0
	for col in 5:
		var s := str(grid[col][line[col]])
		if s == target or s == WILD: count += 1
		else: break
	var ck := str(count)
	if not PAYTABLE[target].has(ck): return {"win":0}
	return {"symbol":target,"count":count,"win":bet * int(PAYTABLE[target][ck])}

func _fortune_info() -> Dictionary:
	return {"id":"fortune-reels","name":"Fortune Reels","cols":5,"rows":3,
		"lines":9,"paylines":PAYLINES_FORTUNE,"wild":WILD,
		"paytable":_pt_dict(),"scatterPays":{"3":4,"4":17,"5":85},
		"minBetPerLine":1,"maxBetPerLine":100}

func _gold_info() -> Dictionary:
	return {"id":"gold-rush","name":"Gold Rush","cols":5,"rows":3,
		"lines":15,"paylines":PAYLINES_GOLD,"wild":WILD,
		"paytable":_pt_dict(),"scatterPays":{"3":1,"4":5,"5":30},
		"minBetPerLine":1,"maxBetPerLine":100}

func _pt_dict() -> Dictionary:
	return PAYTABLE
