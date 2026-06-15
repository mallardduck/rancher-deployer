#!/bin/bash
# Debug script for cert-manager deployment issues

echo "=== Cert-Manager Debug Information ==="
echo

echo "1. Cert-Manager Namespace Status:"
kubectl get ns cert-manager -o yaml 2>/dev/null || echo "  Namespace does not exist"
echo

echo "2. Cert-Manager Deployments:"
kubectl get deployments -n cert-manager -o wide
echo

echo "3. Cert-Manager ReplicaSets:"
kubectl get rs -n cert-manager -o wide
echo

echo "4. Cert-Manager Pods:"
kubectl get pods -n cert-manager -o wide
echo

echo "5. Pod Details (describe):"
for pod in $(kubectl get pods -n cert-manager -o name 2>/dev/null); do
    echo "--- $pod ---"
    kubectl describe $pod -n cert-manager
    echo
done

echo "6. Pod Logs (if any pods exist):"
for pod in $(kubectl get pods -n cert-manager -o name 2>/dev/null); do
    echo "--- Logs for $pod ---"
    kubectl logs $pod -n cert-manager --all-containers=true --tail=50
    echo
done

echo "7. Events in cert-manager namespace:"
kubectl get events -n cert-manager --sort-by='.lastTimestamp'
echo

echo "8. Node Status:"
kubectl get nodes -o wide
kubectl describe nodes
echo

echo "9. All Pods (all namespaces):"
kubectl get pods -A -o wide
echo

echo "10. Container Runtime Info:"
k3s crictl info 2>/dev/null || echo "  Could not get crictl info"
echo

echo "11. Images Available:"
k3s crictl images 2>/dev/null || echo "  Could not list images"
echo

echo "12. Check if cert-manager images are pulling:"
kubectl get events -A | grep -i "pull" | tail -20
echo

echo "=== Common Issues to Check ==="
echo "- Are pods stuck in 'Pending'? Check: kubectl describe pod <pod> -n cert-manager"
echo "- Are pods stuck in 'ImagePullBackOff'? Check: kubectl describe pod <pod> -n cert-manager"
echo "- Are pods 'CrashLoopBackOff'? Check: kubectl logs <pod> -n cert-manager"
echo "- Is the node Ready? Check: kubectl get nodes"
echo "- Is containerd working? Check: k3s crictl ps"
