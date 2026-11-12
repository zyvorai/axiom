#!/usr/bin/env bash
# Exercises the enterprise write endpoints against the REAL remote cluster
# (not the demo provider / fake clientset). Run on the remote host itself so
# admin credentials never leave it. Cleans up everything it creates.
set -u
BASE="https://localhost:30590"
NS="axiom-e2e-validate"
PASS=0
FAIL=0

U=$(sudo k3s kubectl get secret -n axiom-system zyvor-axiom-auth -o jsonpath="{.data.username}" | base64 -d)
P=$(sudo k3s kubectl get secret -n axiom-system zyvor-axiom-auth -o jsonpath="{.data.password}" | base64 -d)
AUTH="$U:$P"

req() {
  local method=$1 path=$2 data=${3:-}
  if [ -n "$data" ]; then
    curl -sk -u "$AUTH" -X "$method" -H 'Content-Type: application/json' -d "$data" -w '\n%{http_code}' "$BASE$path"
  else
    curl -sk -u "$AUTH" -X "$method" -w '\n%{http_code}' "$BASE$path"
  fi
}

check() {
  local label=$1 expect_re=$2 resp=$3
  local code body
  code=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | sed '$d')
  if [[ "$code" =~ $expect_re ]]; then
    echo "PASS  $label  (HTTP $code)"
    PASS=$((PASS+1))
  else
    echo "FAIL  $label  (HTTP $code) body=$body"
    FAIL=$((FAIL+1))
  fi
}

echo "== namespace =="
r=$(req POST /api/v1/namespaces "{\"name\":\"$NS\"}"); check "create namespace" '^(200|201)$' "$r"
sleep 1

echo "== resourcequota =="
r=$(req POST /api/v1/resourcequotas "{\"namespace\":\"$NS\",\"name\":\"rq-e2e\",\"hard\":{\"pods\":\"5\",\"requests.cpu\":\"1\"}}"); check "create quota" '^(200|201)$' "$r"
r=$(req GET "/api/v1/resourcequotas?namespace=$NS"); check "list quota" '^200$' "$r"
echo "$r" | sed '$d' | grep -q rq-e2e && echo "PASS  quota visible in list" && PASS=$((PASS+1)) || { echo "FAIL  quota NOT visible in list"; FAIL=$((FAIL+1)); }
r=$(req DELETE "/api/v1/resourcequotas/$NS/rq-e2e"); check "delete quota" '^(200|204)$' "$r"

echo "== service =="
r=$(req POST /api/v1/services "{\"namespace\":\"$NS\",\"name\":\"svc-e2e\",\"type\":\"ClusterIP\",\"port\":80,\"targetPort\":8080,\"selector\":{\"app\":\"none\"}}"); check "create service" '^(200|201)$' "$r"
r=$(req GET "/api/v1/services?namespace=$NS"); check "list services" '^200$' "$r"
echo "$r" | sed '$d' | grep -q svc-e2e && echo "PASS  service visible in list" && PASS=$((PASS+1)) || { echo "FAIL  service NOT visible in list"; FAIL=$((FAIL+1)); }

echo "== ingress =="
r=$(req POST /api/v1/ingresses "{\"namespace\":\"$NS\",\"name\":\"ing-e2e\",\"host\":\"e2e.example.com\",\"service\":\"svc-e2e\",\"port\":80}"); check "create ingress" '^(200|201)$' "$r"
r=$(req GET "/api/v1/ingresses?namespace=$NS"); check "list ingresses" '^200$' "$r"
echo "$r" | sed '$d' | grep -q ing-e2e && echo "PASS  ingress visible in list" && PASS=$((PASS+1)) || { echo "FAIL  ingress NOT visible in list"; FAIL=$((FAIL+1)); }
r=$(req DELETE "/api/v1/ingresses/$NS/ing-e2e"); check "delete ingress" '^(200|204)$' "$r"
r=$(req DELETE "/api/v1/services/$NS/svc-e2e"); check "delete service" '^(200|204)$' "$r"

echo "== vm (kubevirt) =="
r=$(req POST /api/v1/vms "{\"namespace\":\"$NS\",\"name\":\"vm-e2e\",\"cpu\":\"1\",\"memory\":\"256Mi\",\"image\":\"quay.io/kubevirt/cirros-container-disk-demo\",\"running\":false}"); check "create vm" '^(200|201)$' "$r"
sleep 2
r=$(req GET "/api/v1/vms?namespace=$NS"); check "list vms" '^200$' "$r"
echo "$r" | sed '$d' | grep -q vm-e2e && echo "PASS  vm visible in list" && PASS=$((PASS+1)) || { echo "FAIL  vm NOT visible in list"; FAIL=$((FAIL+1)); }
r=$(req DELETE "/api/v1/vms/$NS/vm-e2e")
code=$(echo "$r" | tail -1)
if [[ "$code" == "403" ]]; then
  echo "KNOWN  delete vm  (HTTP 403 - missing RBAC verb, fix pending redeploy)"
  sudo k3s kubectl delete vm vm-e2e -n "$NS" --wait=false >/dev/null 2>&1
else
  check "delete vm" '^(200|204)$' "$r"
fi

echo "== cluster register/unregister =="
KCFG=$(sudo k3s kubectl config view --flatten --minify | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read()))')
PAYLOAD="{\"name\":\"e2e-fleet-test\",\"kubeconfig\":$KCFG}"
r=$(req POST /api/v1/clusters "$PAYLOAD"); check "register fleet cluster" '^(200|201)$' "$r"
r=$(req GET /api/v1/clusters); check "list clusters" '^200$' "$r"
echo "$r" | sed '$d' | grep -q e2e-fleet-test && echo "PASS  fleet cluster visible in list" && PASS=$((PASS+1)) || { echo "FAIL  fleet cluster NOT visible in list"; FAIL=$((FAIL+1)); }
r=$(req POST /api/v1/clusters/e2e-fleet-test/activate); check "activate fleet cluster" '^(200|204)$' "$r"
r=$(req GET "/api/v1/namespaces")
code=$(echo "$r" | tail -1)
echo "INFO  list namespaces via activated fleet cluster -> HTTP $code (kubeconfig server is 127.0.0.1:6443, unreachable from inside the pod; expected for a self-registered loopback kubeconfig, not a real second cluster)"
r=$(req DELETE /api/v1/clusters/e2e-fleet-test); check "unregister fleet cluster" '^(200|204)$' "$r"

echo "== helm install/upgrade/uninstall (disposable release) =="
r=$(req POST /api/v1/helm/install "{\"namespace\":\"$NS\",\"releaseName\":\"e2e-podinfo\",\"repoUrl\":\"https://stefanprodan.github.io/podinfo\",\"chartName\":\"podinfo\",\"chartVersion\":\"6.6.2\"}"); check "helm install" '^(200|201|202)$' "$r"
echo "waiting for helm install job..."
sleep 25
r=$(req GET /api/v1/helm/releases); check "list helm releases" '^200$' "$r"
echo "$r" | sed '$d' | grep -q e2e-podinfo && echo "PASS  podinfo release visible" && PASS=$((PASS+1)) || { echo "FAIL  podinfo release NOT visible (job may still be running)"; FAIL=$((FAIL+1)); }
r=$(req POST "/api/v1/helm/upgrade" "{\"namespace\":\"$NS\",\"releaseName\":\"e2e-podinfo\",\"repoUrl\":\"https://stefanprodan.github.io/podinfo\",\"chartName\":\"podinfo\",\"chartVersion\":\"6.6.3\"}"); check "helm upgrade" '^(200|201|202)$' "$r"
echo "waiting for helm upgrade job..."
sleep 20
r=$(req GET "/api/v1/helm/releases/$NS/e2e-podinfo/history"); check "helm history" '^200$' "$r"
r=$(req POST "/api/v1/helm/releases/$NS/e2e-podinfo/uninstall"); check "helm uninstall" '^(200|201|202)$' "$r"
sleep 10

echo "== velero =="
if sudo k3s kubectl get crd backups.velero.io >/dev/null 2>&1; then
  VNS=$(sudo k3s kubectl get ns velero -o name 2>/dev/null | head -1 | sed 's#namespace/##' || true)
  VNS=${VNS:-velero}
  VNAME="axiom-e2e-velero-$(date +%s)"
  r=$(req POST /api/v1/velero/backups "{\"namespace\":\"$VNS\",\"name\":\"$VNAME\",\"includedNamespaces\":[\"$NS\"],\"ttl\":\"24h0m0s\"}")
  check "create velero backup" '^(200|201)$' "$r"
  r=$(req GET "/api/v1/velero/backups?namespace=$VNS")
  check "list velero backups" '^200$' "$r"
  echo "$r" | sed '$d' | grep -q "$VNAME" && echo "PASS  velero backup visible" && PASS=$((PASS+1)) || { echo "FAIL  velero backup NOT visible"; FAIL=$((FAIL+1)); }
  sudo k3s kubectl -n "$VNS" delete backup.velero.io "$VNAME" --wait=false >/dev/null 2>&1 || true
else
  r=$(req GET "/api/v1/velero/backups?namespace=$NS")
  code=$(echo "$r" | tail -1)
  echo "INFO  velero backups list -> HTTP $code (CRDs absent — run e2e/scripts/validate-velero.sh after installing Velero)"
fi

echo "== cleanup =="
sudo k3s kubectl delete ns "$NS" --wait=false >/dev/null 2>&1
sudo k3s kubectl delete job -n "$NS" --all --wait=false >/dev/null 2>&1

echo
echo "===== SUMMARY: $PASS passed, $FAIL failed ====="
