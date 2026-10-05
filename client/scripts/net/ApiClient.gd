# ApiClient: thin wrapper over the slots-engine HTTP API
# (server side: internal/api/*.go — if an endpoint changes, this is the one
# client file to update).
#
# Every method is a coroutine; callers must `await` it. Every method
# returns a Dictionary:
#   ok:   bool    true if HTTP status was 2xx
#   code: int     HTTP status (0 = could not reach the server)
#   data: Variant parsed JSON body (Dictionary/Array) or null
#   raw:  String  raw body / local error text, for messages
class_name ApiClient
extends Node

var base_url: String = ""
var player_id: String = ""
var token: String = ""

func _request(method: int, path: String, body: Variant = null) -> Dictionary:
	if base_url == "":
		return {"ok": false, "code": 0, "data": null, "raw": "No server address configured"}
	var http := HTTPRequest.new()
	http.timeout = 12.0
	add_child(http)

	var headers := PackedStringArray(["Content-Type: application/json"])
	if token != "":
		headers.append("Authorization: Bearer %s" % token)

	var body_str := ""
	if method == HTTPClient.METHOD_POST:
		# Always send a JSON object on POST (the server rejects an empty body).
		body_str = JSON.stringify(body if body != null else {})

	var err := http.request(base_url + path, headers, method, body_str)
	if err != OK:
		http.queue_free()
		return {"ok": false, "code": 0, "data": null, "raw": "Could not start request (error %d)" % err}

	var result: Array = await http.request_completed
	http.queue_free()

	var transport_result: int = result[0]
	var code: int = result[1]
	var text: String = (result[3] as PackedByteArray).get_string_from_utf8()
	if transport_result != HTTPRequest.RESULT_SUCCESS:
		return {"ok": false, "code": 0, "data": null, "raw": "Network error (%d)" % transport_result}

	var data: Variant = null
	if text != "":
		var json := JSON.new()
		if json.parse(text) == OK:
			data = json.data
	return {"ok": code >= 200 and code < 300, "code": code, "data": data, "raw": text}

func _pp(suffix: String) -> String:
	return "/v1/players/%s/%s" % [player_id, suffix]

func health_check() -> Dictionary:
	return await _request(HTTPClient.METHOD_GET, "/v1/healthz")

func guest() -> Dictionary:
	return await _request(HTTPClient.METHOD_POST, "/v1/auth/guest")

func games() -> Dictionary:
	return await _request(HTTPClient.METHOD_GET, "/v1/games")

func packages() -> Dictionary:
	return await _request(HTTPClient.METHOD_GET, "/v1/store/packages")

func get_balance() -> Dictionary:
	return await _request(HTTPClient.METHOD_GET, _pp("balance"))

func purchase(package_id: String) -> Dictionary:
	return await _request(HTTPClient.METHOD_POST, _pp("demo-purchase"), {"packageId": package_id})

func daily_status() -> Dictionary:
	return await _request(HTTPClient.METHOD_GET, _pp("daily-bonus"))

func daily_claim() -> Dictionary:
	return await _request(HTTPClient.METHOD_POST, _pp("daily-bonus"))

## idempotency_key must be unique per spin attempt and REUSED when retrying
## the same attempt — that is what makes a retry safe from a double charge.
func spin(game_id: String, bet_per_line: int, lines: int, idempotency_key: String) -> Dictionary:
	return await _request(HTTPClient.METHOD_POST, _pp("spin"), {
		"gameId": game_id,
		"betPerLine": bet_per_line,
		"linesPlayed": lines,
		"idempotencyKey": idempotency_key,
	})

func new_idempotency_key() -> String:
	return "spin-%d-%d-%d" % [Time.get_ticks_usec(), randi(), randi()]
