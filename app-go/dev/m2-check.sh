#!/usr/bin/env bash
# M2 check (spec §15.3 I13): walks through every proxy row of spec §12 against a running
# tmock and echo-upstream, printing what to expect next to what happened.
#
#   go run ./cmd/echo-upstream &          # localhost:8081
#   make run                              # localhost:3000, in another terminal
#   TMOCK_EMAIL=you@example.com TMOCK_PASSWORD=… dev/m2-check.sh
#
# It creates the service "m2" with its scenarios and the clients "m2-qa" and "m2-v6", and
# deletes them again at the end. Needs curl and jq. The rows that need other settings
# (PROXY_REMOVE_FORWARDED_HEADERS=true, PROXY_CORS_ALLOWED_ORIGINS, PROXY_UPSTREAM_TIMEOUT)
# are listed at the end, to try by restarting tmock.
set -euo pipefail

B=${TMOCK_URL:-http://localhost:3000}
UPSTREAM=${ECHO_URL:-http://localhost:8081}
: "${TMOCK_EMAIL:?set TMOCK_EMAIL and TMOCK_PASSWORD to an account of the running tmock}"
: "${TMOCK_PASSWORD:?set TMOCK_PASSWORD}"
J=$(mktemp)
P=$B/api/proxy/m2
QA=(-H 'X-Real-Ip: 10.0.0.1')

api() { curl -s -b "$J" -c "$J" -H 'Content-Type: application/json' "$@"; }
heads() { curl -s -o /dev/null -D - "$@" | tr -d '\r'; }
say() { printf '\n== %s\n   expect: %s\n' "$1" "$2"; }

# remove_data deletes what the check creates (the service's scenarios go with it).
remove_data() {
	for id in $(api "$B/api/client" | jq -r '.[] | select(.name | startswith("m2-")) | .id'); do
		api -X DELETE "$B/api/client/$id" >/dev/null
	done
	api -X DELETE "$B/api/service/m2" >/dev/null
}

code=$(api -o /dev/null -w '%{http_code}' -X POST "$B/api/auth/login" \
	-d "$(jq -n --arg e "$TMOCK_EMAIL" --arg p "$TMOCK_PASSWORD" '{email:$e,password:$p}')")
[ "$code" = 200 ] || { echo "login failed: $code" >&2; rm -f "$J"; exit 1; }
trap 'remove_data; rm -f "$J"' EXIT
remove_data # leftovers of an interrupted run
api -X POST "$B/api/service" -d "{\"path\":\"m2\",\"name\":\"M2 check\",\"upstreamUrl\":\"$UPSTREAM/base\"}" >/dev/null

scenario() { api -X POST "$B/api/scenario" -d "$1" | jq -r '.id // .'; }
id_cond=$(scenario '{"service":"m2","name":"m-cond","requestMethod":"POST","requestPath":"pay","requestCondition":"$request.body.userId == \"42\"","skipProxy":true,"responseBody":"{\"paid\":true}"}')
id_a=$(scenario '{"service":"m2","name":"a-first","requestPath":"order","skipProxy":true,"responseBody":"first"}')
id_b=$(scenario '{"service":"m2","name":"b-second","requestPath":"order","skipProxy":true,"responseBody":"second"}')
id_t=$(scenario '{"service":"m2","name":"t-now","requestPath":"orders","skipProxy":true,"responseBody":"<% TEMPLATE: LiquidJS %>{{ \"now\" | date: \"%s%L\" }}"}')
id_h=$(scenario '{"service":"m2","name":"h-headers","requestPath":"profile","responseHeaders":{"X-Mock":"1"}}')
api -X POST "$B/api/client" -d "{\"name\":\"m2-qa\",\"enabled\":true,\"condition\":{\"or\":[{\"ip\":\"10.0.0.1\"},{\"headerRegex\":{\"header\":\"User-Agent\",\"value\":\"^Mobile\\\\d+\$\"}}]},\"scenarios\":[$id_cond,$id_a,$id_b,$id_t,$id_h]}" >/dev/null
api -X POST "$B/api/client" -d "{\"name\":\"m2-v6\",\"enabled\":true,\"condition\":{\"cidr\":\"2001:db8::/32\"},\"scenarios\":[$id_a]}" >/dev/null

say "Request conditions: no eval" "400 with a column and a hint, for both"
api -X POST "$B/api/scenario" -d '{"service":"m2","name":"x","requestCondition":"$request.body.a = 1"}' | jq -c .details
api -X POST "$B/api/scenario" -d '{"service":"m2","name":"x","requestCondition":"process.exit(1)"}' | jq -c .details
say "Request conditions: a JSON body" '{"paid":true}, then the echo of /base/pay'
curl -s -X POST "$P/pay" "${QA[@]}" -H 'Content-Type: application/json' -d '{"userId":"42"}'; echo
curl -s -X POST "$P/pay" "${QA[@]}" -H 'Content-Type: application/json' -d '{"userId":"7"}' | jq -c '{url,body}'

say "Templates: rendered on every request" "two different millisecond times"
curl -s "$P/orders" "${QA[@]}"; echo
curl -s "$P/orders" "${QA[@]}"; echo

say "Scenario order by name; cache invalidated after a change" "first, then (a-first renamed to z-first) second"
curl -s "$P/order" "${QA[@]}"; echo
api -X PATCH "$B/api/scenario/$id_a" -d '{"name":"z-first"}' >/dev/null
curl -s "$P/order" "${QA[@]}"; echo

say "Path matching: without query, slashes or case" "t-now three times, then no scenario"
for u in 'orders?page=2' 'orders/' 'ORDERS' 'orders/1'; do
	printf '   %-14s ' "$u"
	heads "$P/$u" "${QA[@]}" | grep -i '^x-tmock-scenario' || echo '(no scenario)'
done

say "Headers: x-tmock-* on a skip-proxy response" "status mocked, client m2-qa, scenario b-second"
heads "$P/order" "${QA[@]}" | grep -i '^x-tmock'

say "Conditions: RE2 regex, IPv6 CIDR" "a 400 for (?= ; h-headers by User-Agent; m2-v6 by 2001:db8::5"
api -X POST "$B/api/client" -d '{"name":"m2-js","condition":{"headerRegex":{"header":"User-Agent","value":"^(?=Mobile)"}}}' | jq -c .details
heads "$P/profile" -H 'X-Real-Ip: 192.0.2.1' -H 'User-Agent: Mobile5' | grep -i '^x-tmock-scenario\|^x-mock'
heads "$P/order" -H 'X-Real-Ip: 2001:db8::5' | grep -i '^x-tmock-client'

say "Scenario headers: framing headers are tmock's (I13)" "400 for Content-Length"
api -X POST "$B/api/scenario" -d '{"service":"m2","name":"x","responseHeaders":{"Content-Length":"3"}}' | jq -c .details

say "PROXY_REMOVE_FORWARDED_HEADERS (as configured)" "the headers, unless it is true"
curl -s "$P/fw" -H 'X-Forwarded-Host: shop.example' -H 'X-Scheme: https' | jq -c '.headers | {"X-Forwarded-Host","X-Scheme"}'

say "Proxy CORS: proxy only" "204 with the origin reflected (if allowed); 0 CORS headers on the API"
heads -X OPTIONS "$P/x" -H 'Origin: http://app.test' -H 'Access-Control-Request-Method: PUT' | grep -i '^HTTP\|^access-control-allow-origin'
printf '   API CORS headers: '; heads "$B/api/status" -H 'Origin: http://app.test' | grep -ci '^access-control' || true

cat <<'EOT'

== By hand, restarting tmock with other settings:
   PROXY_REMOVE_FORWARDED_HEADERS=maybe     → refuses to start
   PROXY_REMOVE_FORWARDED_HEADERS=true      → the forwarded headers above are null
   PROXY_CORS_ALLOWED_ORIGINS=http://a.test → an Origin not in the list is not reflected
   PROXY_UPSTREAM_TIMEOUT=1s + echo-upstream -delay 3s → 502 after 1s, error.type=timeout
EOT
