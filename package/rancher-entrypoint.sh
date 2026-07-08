#!/bin/bash
# Wrapper entrypoint for rancher-deployer target
# Starts k3s-base, waits for ready, then deploys Rancher

set -e

echo "=== Rancher Deployer Entrypoint ==="
echo "Rancher version: ${RANCHER_VERSION:-2.14.2}"

# ============================================================================
# Environment Variable Configuration
# ============================================================================
# The following environment variables can be set to customize the deployment:
#
#   RANCHER_VERSION             - Rancher version to deploy (default: 2.14.2)
#   RANCHER_INGRESS_ENABLED     - Enable ingress with hostname restrictions (default: false)
#                                 When false (default): Direct service access, no hostname restrictions
#                                 When true: Creates ingress with specific hostname (like k3d deployments)
#   RANCHER_HOSTNAME            - Hostname for Rancher ingress (default: auto-detected IP.sslip.io)
#                                 Only used when RANCHER_INGRESS_ENABLED=true
#   RANCHER_BOOTSTRAP_PASSWORD  - Initial admin password (default: letsmein)
#   RANCHER_NAMESPACE           - Kubernetes namespace (default: cattle-system)
#   RANCHER_VALUES_FILE         - Path to Helm values YAML file (default: none)
#   RANCHER_HELM_SET            - Comma-separated Helm --set values (default: none)
#                                 Example: "replicas=3,auditLog.level=1"
#   RANCHER_PRIME               - Use Rancher Prime edition (default: false)
#   RANCHER_CHANNEL             - Release channel: stable, latest, alpha (default: stable)
#   K8S_VERSION                 - Target k8s major.minor version (default: auto)
#   CATTLE_NAMESPACE            - (deprecated, use RANCHER_NAMESPACE)
# ============================================================================

# Resolve the correct k3s version for the desired Rancher version
if [ -z "$CATTLE_K3S_VERSION" ]; then
    echo "Resolving k3s version for Rancher ${RANCHER_VERSION}..."
    CATTLE_K3S_VERSION=$(rancher-deployer resolve \
        --rancher-version "${RANCHER_VERSION}" \
        --mode k3s \
        --output k3s-version)

    if [ -z "$CATTLE_K3S_VERSION" ]; then
        echo "ERROR: Failed to resolve k3s version for Rancher ${RANCHER_VERSION}"
        exit 1
    fi
    echo "Resolved k3s version: ${CATTLE_K3S_VERSION} (from Rancher KDM)"
    export CATTLE_K3S_VERSION
else
    echo "Using user-specified k3s version: ${CATTLE_K3S_VERSION}"
fi

# Start k3s in background using k3s-base entrypoint
echo "Starting k3s..."
/entrypoint.sh > /var/log/k3s.log 2>&1 &
K3S_PID=$!

# Give k3s a moment to initialize
sleep 5

# Wait for k3s API server
echo "Waiting for k3s API server..."
WAIT_COUNT=0
MAX_WAIT=60
until kubectl get nodes >/dev/null 2>&1; do
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $WAIT_COUNT -gt $MAX_WAIT ]; then
        echo "ERROR: k3s API server failed to start"
        echo "k3s logs:"
        cat /var/log/k3s.log
        exit 1
    fi

    if ! kill -0 $K3S_PID 2>/dev/null; then
        echo "ERROR: k3s process died"
        echo "k3s logs:"
        cat /var/log/k3s.log
        exit 1
    fi

    sleep 2
done

# Wait for node to be Ready
echo "Waiting for k3s node to be Ready..."
WAIT_COUNT=0
MAX_WAIT=60
until kubectl get nodes | grep -w "Ready" >/dev/null 2>&1; do
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $WAIT_COUNT -gt $MAX_WAIT ]; then
        echo "ERROR: k3s node did not become Ready"
        kubectl get nodes -o wide || true
        cat /var/log/k3s.log
        exit 1
    fi

    echo "Node status: $(kubectl get nodes --no-headers 2>/dev/null | awk '{print $2}') (${WAIT_COUNT}s)"
    sleep 2
done
echo "k3s node is Ready!"

# Wait for core components
echo "Waiting for core k3s components..."
kubectl wait --for=condition=ready pod -l k8s-app=kube-dns -n kube-system --timeout=120s || \
    echo "WARNING: coredns not ready, but continuing"

# Show cluster info
echo "k3s cluster ready:"
kubectl get nodes -o wide
kubectl get pods -A -o wide

# Cleanup handler
cleanup() {
    echo "Shutting down..."
    kill $K3S_PID 2>/dev/null || true
    wait $K3S_PID 2>/dev/null || true
}
trap cleanup EXIT TERM INT

# Check if Rancher is already deployed (for container restarts with persistent volumes)
echo "=== Checking for existing Rancher installation ==="
RANCHER_DEPLOYED=false

# Check if Helm release exists
if helm list -n cattle-system --short 2>/dev/null | grep -q "^rancher$"; then
    echo "Found existing Helm release 'rancher' in cattle-system namespace"
    RANCHER_DEPLOYED=true
fi

# Double-check with deployment
if kubectl get deployment rancher -n cattle-system >/dev/null 2>&1; then
    echo "Found existing Rancher deployment"
    RANCHER_DEPLOYED=true
fi

if [ "$RANCHER_DEPLOYED" = "true" ]; then
    echo ""
    echo "=== Rancher Already Deployed (Skipping Deployment) ==="
    echo ""
    echo "Detected existing Rancher installation - this is likely a container restart."
    echo "Skipping deployment and monitoring existing installation."
    echo ""
    echo "To do a clean redeployment:"
    echo "  1. Stop and remove the container: docker rm -f <container-name>"
    echo "  2. Delete the volume: docker volume rm <volume-name>"
    echo "  3. Start fresh: docker run -d --privileged -p 80:80 -p 443:443 -v new-volume:/var/lib/rancher rancher-deployer:latest"
    echo ""

    # Show current status
    echo "Current Rancher status:"
    kubectl get deployment rancher -n cattle-system -o wide 2>/dev/null || true
    kubectl get pods -n cattle-system -l app=rancher 2>/dev/null || true
    echo ""

    # Display access URL (check if ingress exists)
    if kubectl get ingress -n cattle-system rancher >/dev/null 2>&1; then
        RANCHER_HOST=$(kubectl get ingress -n cattle-system rancher -o jsonpath='{.spec.rules[0].host}' 2>/dev/null || echo 'rancher.local')
        echo "Rancher UI: https://${RANCHER_HOST}"
    else
        echo "Rancher UI: https://<docker-host-ip>"
        echo "  (Ingress disabled - accessible via any hostname or IP)"
    fi
    echo ""
else
    # Fresh deployment
    echo "No existing Rancher installation found - proceeding with deployment"
    echo ""
    echo "=== Deploying Rancher ${RANCHER_VERSION} ==="

    # Build deployment command from environment variables
    DEPLOY_ARGS=(
        "deploy"
        "--mode" "existing"
        "--rancher-version" "${RANCHER_VERSION}"
        "--yes"
    )

    # Optional: Ingress configuration (default: disabled for Docker-like access)
    if [ "$RANCHER_INGRESS_ENABLED" = "true" ] || [ "$RANCHER_INGRESS_ENABLED" = "1" ]; then
        echo "Ingress enabled with hostname restrictions"
        # Optional: Custom hostname (only used when ingress is enabled)
        if [ -n "$RANCHER_HOSTNAME" ]; then
            DEPLOY_ARGS+=("--hostname" "$RANCHER_HOSTNAME")
            echo "Using custom hostname: $RANCHER_HOSTNAME"
        fi
    else
        # Disable ingress for direct service access (matches old rancher/rancher docker mode)
        DEPLOY_ARGS+=("--disable-ingress")
        echo "Ingress disabled - Rancher accessible via direct service access (any hostname/IP)"
    fi

    # Optional: Bootstrap password
    if [ -n "$RANCHER_BOOTSTRAP_PASSWORD" ]; then
        DEPLOY_ARGS+=("--bootstrap-password" "$RANCHER_BOOTSTRAP_PASSWORD")
        echo "Using custom bootstrap password"
    fi

    # Optional: Namespace (support both RANCHER_NAMESPACE and legacy CATTLE_NAMESPACE)
    NAMESPACE="${RANCHER_NAMESPACE:-${CATTLE_NAMESPACE}}"
    if [ -n "$NAMESPACE" ] && [ "$NAMESPACE" != "cattle-system" ]; then
        DEPLOY_ARGS+=("--namespace" "$NAMESPACE")
        echo "Using namespace: $NAMESPACE"
    fi

    # Optional: Values file
    if [ -n "$RANCHER_VALUES_FILE" ]; then
        if [ -f "$RANCHER_VALUES_FILE" ]; then
            DEPLOY_ARGS+=("--values-file" "$RANCHER_VALUES_FILE")
            echo "Using Helm values file: $RANCHER_VALUES_FILE"
        else
            echo "WARNING: RANCHER_VALUES_FILE set but file not found: $RANCHER_VALUES_FILE"
        fi
    fi

    # Optional: Helm set values (comma-separated)
    if [ -n "$RANCHER_HELM_SET" ]; then
        # Split comma-separated values and add each as a --set flag
        IFS=',' read -ra HELM_SETS <<< "$RANCHER_HELM_SET"
        for set_value in "${HELM_SETS[@]}"; do
            DEPLOY_ARGS+=("--set" "$set_value")
        done
        echo "Using Helm --set values: $RANCHER_HELM_SET"
    fi

    # Optional: Rancher Prime
    if [ "$RANCHER_PRIME" = "true" ] || [ "$RANCHER_PRIME" = "1" ]; then
        DEPLOY_ARGS+=("--prime")
        echo "Using Rancher Prime edition"
    fi

    # Optional: Release channel
    if [ -n "$RANCHER_CHANNEL" ] && [ "$RANCHER_CHANNEL" != "stable" ]; then
        DEPLOY_ARGS+=("--channel" "$RANCHER_CHANNEL")
        echo "Using release channel: $RANCHER_CHANNEL"
    fi

    # Optional: K8s version
    if [ -n "$K8S_VERSION" ]; then
        DEPLOY_ARGS+=("--k8s-version" "$K8S_VERSION")
        echo "Using k8s version: $K8S_VERSION"
    fi

    # Append any additional args passed to the container
    DEPLOY_ARGS+=("$@")

    echo ""
    echo "Executing: rancher-deployer ${DEPLOY_ARGS[*]}"
    echo ""

    # Execute deployment
    rancher-deployer "${DEPLOY_ARGS[@]}"

    DEPLOY_EXIT=$?
    if [ $DEPLOY_EXIT -ne 0 ]; then
        echo "ERROR: Rancher deployment failed with exit code $DEPLOY_EXIT"
        exit $DEPLOY_EXIT
    fi

    echo ""
    echo "=== Rancher Deployment Complete ==="
fi
echo ""
if [ "$RANCHER_DEPLOYED" = "false" ]; then
    echo "Rancher is now running!"
else
    echo "Rancher is running (existing installation)"
fi

# Display access URL based on ingress configuration
if [ "$RANCHER_INGRESS_ENABLED" = "true" ] || [ "$RANCHER_INGRESS_ENABLED" = "1" ]; then
    RANCHER_HOST=$(kubectl get ingress -n cattle-system rancher -o jsonpath='{.spec.rules[0].host}' 2>/dev/null || echo 'rancher.local')
    echo "Access the UI at: https://${RANCHER_HOST}"
else
    echo "Access the UI at: https://<docker-host-ip>"
    echo "  (Ingress disabled - accessible via any hostname or IP)"
fi
echo ""
echo "Get bootstrap password:"
echo "  kubectl get secret --namespace cattle-system bootstrap-secret -o go-template='{{.data.bootstrapPassword|base64decode}}{{\"\\n\"}}'"
echo ""
echo "Useful commands:"
echo "  docker exec -it <container> k9s                    # Interactive cluster view"
echo "  docker exec -it <container> debug-rancher          # Debug Rancher deployment"
echo "  docker exec -it <container> kubectl get pods -A    # List all pods"
echo ""
echo "Container will continue running with k3s in the background."
echo ""

# Keep container alive - wait for k3s process
# If k3s dies, the container should exit
echo "Keeping container alive, monitoring k3s (PID $K3S_PID)..."
wait $K3S_PID
EXIT_CODE=$?
echo "k3s exited with code $EXIT_CODE"
exit $EXIT_CODE
