# ApiClient: thin HTTP wrapper around the slots-engine backend.
# When the server isn't reachable (web demo, no backend configured) it
# transparently falls back to MockServer so the demo always works.
class_name ApiClient
extends Node

var base_url: String = ""
var player_id: String = ""
var token: String = ""

var _mock: MockServer = null
var _use_mock: bool = false

func _request(method: int, path: String, body: Variant = null) -> Dictionary:
	if _use_mock:
		return _dispatch_mock(path, method, body)
	if base_url == "":
		_use_mock = true
		return _dispatch_mock(path, method, body)

	var http := HTTPRequest.new()
	http.timeout = 10.0
	add_child(http)

	var headers := PackedStringArray(["Content-Type: application/json"])
	if token != "":
		headers.append("Authorization: Bearer %s" % token)

	var body_str := ""
	if method == HTTPClient.METHOD_POST:
		body_str = JSON.stringify(body if body != null else {})

	var err := http.request(base_url + path, headers, method, body_str)
	if err != OK:
		http.queue_free()
		# Network error → switch to mock for the rest of the session
		_use_mock = true
		_init_mock()
		return _dispatch_mock(path, method, body)

	var result: Array = await http.request_completed
	http.queue_free()

	var transport_err: int = result[0]
	var code: int = result[1]
	var text: String = (result[3] as PackedByteArray).get_string_from_utf8()

	if transport_err != HTTPRequest.RESULT_SUCCESS or code == 0:
		_use_mock = true
		_init_mock()
		return _dispatch_mock(path, method, body)

	var data: Variant = null
	if text != "":
		var json := JSON.new()
		if json.parse(text) == OK:
			data = json.data
	return {"ok": code >= 200 and code < 300, "code": code, "data": data, "raw": text}

func _init_mock() -> void:
	if _mock == null:
		_mock = MockServer.new()

func _dispatch_mock(path: String, method: int, body: Variant) -> Dictionary:
	_init_mock()
	if "/healthz" in path: return _mock.health()
	if "/auth/guest" in path: player_id = ""; return _mock.guest()
	if "/balance" in path: return _mock.get_balance()
	if "/games" in path: return _mock.games()
	if "/store/packages" in path: return _mock.packages()
	if "/demo-purchase" in path:
		return _mock.purchase(str((body as Dictionary).get("packageId","starter")))
	if "/daily-bonus" in path:
		return _mock.daily_claim() if method == HTTPClient.METHOD_POST else _mock.daily_status()
	if "/spin" in path and body is Dictionary:
		return _mock.spin(str(body.get("gameId","fortune-reels")),
			int(body.get("betPerLine",1)), int(body.get("linesPlayed",9)),
			str(body.get("idempotencyKey",str(Time.get_ticks_usec()))))
	return {"ok":false,"code":404,"data":null,"raw":"mock: unknown route "+path}

func is_mock() -> bool: return _use_mock

func _pp(s: String) -> String:
	return "/v1/players/%s/%s" % [player_id, s]

func health_check() -> Dictionary: return await _request(HTTPClient.METHOD_GET, "/v1/healthz")
func guest() -> Dictionary: return await _request(HTTPClient.METHOD_POST, "/v1/auth/guest")
func games() -> Dictionary: return await _request(HTTPClient.METHOD_GET, "/v1/games")
func packages() -> Dictionary: return await _request(HTTPClient.METHOD_GET, "/v1/store/packages")
func get_balance() -> Dictionary: return await _request(HTTPClient.METHOD_GET, _pp("balance"))
func purchase(pkg_id: String) -> Dictionary: return await _request(HTTPClient.METHOD_POST, _pp("demo-purchase"), {"packageId":pkg_id})
func daily_status() -> Dictionary: return await _request(HTTPClient.METHOD_GET, _pp("daily-bonus"))
func daily_claim() -> Dictionary: return await _request(HTTPClient.METHOD_POST, _pp("daily-bonus"))
func spin(game_id:String, bpl:int, lines:int, key:String) -> Dictionary:
	return await _request(HTTPClient.METHOD_POST, _pp("spin"), {"gameId":game_id,"betPerLine":bpl,"linesPlayed":lines,"idempotencyKey":key})
func new_idempotency_key() -> String:
	return "spin-%d-%d-%d" % [Time.get_ticks_usec(), randi(), randi()]
