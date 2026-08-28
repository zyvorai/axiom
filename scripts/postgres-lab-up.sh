#!/usr/bin/env bash
# Deploy Axiom postgres-lab into axiom-system (password secret + Deployment + Service).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
HOST="${1:-}"
USER="${2:-sus}"
PASS="${3:-}"

run_kubectl() {
  local args="$*"
  if [[ -n "$HOST" ]]; then
    if [[ -n "$PASS" ]]; then
      SSHPASS="$PASS" sshpass -e ssh -o BatchMode=no -o StrictHostKeyChecking=accept-new "${USER}@${HOST}" \
        "if [ -r \"\$HOME/.kube/config\" ]; then export KUBECONFIG=\"\$HOME/.kube/config\"; fi; sudo k3s kubectl ${args}"
    else
      ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "${USER}@${HOST}" \
        "if [ -r \"\$HOME/.kube/config\" ]; then export KUBECONFIG=\"\$HOME/.kube/config\"; fi; sudo k3s kubectl ${args}"
    fi
  else
    kubectl "$@"
  fi
}

PW="$(openssl rand -base64 18 | tr -d '/+=' | head -c 24)"
echo "Creating postgres-lab-auth in axiom-system…"
if [[ -n "$HOST" ]]; then
  ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "${USER}@${HOST}" \
    "sudo k3s kubectl -n axiom-system create secret generic postgres-lab-auth \
      --from-literal=password='${PW}' \
      --from-literal=username=axiom \
      --from-literal=database=axiom \
      --dry-run=client -o yaml | sudo k3s kubectl apply -f -"
else
  kubectl -n axiom-system create secret generic postgres-lab-auth \
    --from-literal=password="$PW" \
    --from-literal=username=axiom \
    --from-literal=database=axiom \
    --dry-run=client -o yaml | kubectl apply -f -
fi

MANIFEST="$REPO_DIR/deploy/postgres-lab/deployment.yaml"
if [[ -n "$HOST" ]]; then
  scp -o StrictHostKeyChecking=accept-new "$MANIFEST" "${USER}@${HOST}:/tmp/axiom-postgres-lab.yaml"
  run_kubectl apply -f /tmp/axiom-postgres-lab.yaml
else
  run_kubectl apply -f "$MANIFEST"
fi

echo "Waiting for postgres-lab…"
run_kubectl -n axiom-system rollout status deploy/postgres-lab --timeout=180s
echo "postgres-lab Ready."
echo "Host: postgres-lab.axiom-system.svc:5432  user=axiom  db=axiom"
if [[ -n "$HOST" ]]; then
  echo "Fetch password: ssh ${USER}@${HOST} \"sudo k3s kubectl -n axiom-system get secret postgres-lab-auth -o jsonpath='{.data.password}' | base64 -d; echo\""
else
  echo "Password: kubectl -n axiom-system get secret postgres-lab-auth -o jsonpath='{.data.password}' | base64 -d; echo"
fi
