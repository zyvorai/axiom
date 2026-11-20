#!/usr/bin/env sh
set -eu
BASE="${BASE:-http://localhost:5090}"
AUTH="${AUTH:-demo:demo}"

curl -fsS "$BASE/api/v1/overview" -u "$AUTH"; echo
curl -fsS "$BASE/api/v1/workloads" -u "$AUTH"; echo
curl -fsS -X POST "$BASE/api/v1/workloads" -u "$AUTH" \
  -H 'Content-Type: application/json' \
  -d '{"namespace":"apps","name":"hello-axiom","image":"nginx:1.27","replicas":3,"port":80,"expose":true}'; echo
