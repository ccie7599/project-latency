#!/bin/bash
set -euo pipefail

# Deploy latency agent to all LKE clusters
# Usage: ./deploy-agents.sh

LINODE_TOKEN=$(cat ~/project-landing-zone/presales-landing-zone/.linode-token | grep -v '^#' | tr -d '\n')
HUB_IP="172.238.169.30"
CERT_DIR="$HOME/.acme.sh/*.connected-cloud.io_ecc"
TEMPLATE="/home/bapley/project-latency/infra/k8s/agent-deployment.yaml.tmpl"
KUBECONFIG_DIR="/tmp/latency-kubeconfigs"

mkdir -p "$KUBECONFIG_DIR"

# Base64 encode TLS cert and key
TLS_CRT_B64=$(base64 -w0 "${CERT_DIR}/fullchain.cer")
TLS_KEY_B64=$(base64 -w0 "${CERT_DIR}/*.connected-cloud.io.key")

# Get all latency LKE clusters
echo "=== Fetching LKE clusters ==="
CLUSTERS=$(curl -s -H "Authorization: Bearer $LINODE_TOKEN" \
    https://api.linode.com/v4/lke/clusters | \
    python3 -c "import json,sys; [print(f'{c[\"id\"]} {c[\"label\"]} {c[\"region\"]}') for c in json.load(sys.stdin).get('data',[]) if c['label'].startswith('latency-')]")

echo "$CLUSTERS" | while read CID CLABEL CREGION; do
    echo -n "$CLABEL ($CREGION): "

    # Get kubeconfig
    KC_FILE="${KUBECONFIG_DIR}/${CLABEL}.yaml"
    curl -s -H "Authorization: Bearer $LINODE_TOKEN" \
        "https://api.linode.com/v4/lke/clusters/${CID}/kubeconfig" | \
        python3 -c "import json,sys,base64; print(base64.b64decode(json.load(sys.stdin)['kubeconfig']).decode())" > "$KC_FILE" 2>/dev/null

    if [ ! -s "$KC_FILE" ]; then
        echo "FAILED (kubeconfig)"
        continue
    fi

    # Extract region ID from cluster label (latency-xxx -> region from terraform)
    # We need the actual Linode region ID, which is in CREGION
    REGION="$CREGION"

    # Generate manifest from template
    MANIFEST="/tmp/agent-${CLABEL}.yaml"
    sed -e "s|__REGION__|${REGION}|g" \
        -e "s|__HUB_IP__|${HUB_IP}|g" \
        -e "s|__TLS_CRT_B64__|${TLS_CRT_B64}|g" \
        -e "s|__TLS_KEY_B64__|${TLS_KEY_B64}|g" \
        "$TEMPLATE" > "$MANIFEST"

    # Deploy
    if KUBECONFIG="$KC_FILE" kubectl apply -f "$MANIFEST" > /dev/null 2>&1; then
        echo "OK"
    else
        echo "FAILED (kubectl)"
    fi

    rm -f "$MANIFEST"
done

echo ""
echo "=== Deployment complete ==="
echo "Check hub routes: curl -sk https://${HUB_IP}:8222/routez | python3 -c \"import json,sys; rz=json.load(sys.stdin); print(f'Routes: {rz[\\\"num_routes\\\"]}')\""
