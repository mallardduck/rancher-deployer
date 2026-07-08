#!/bin/bash
# Debug script for Rancher Helm installation issues

echo "=== Rancher Helm Debug Information ==="
echo

echo "1. Helm Release Status:"
helm list -n cattle-system -a
echo

echo "2. Helm Release Details:"
helm status rancher -n cattle-system 2>/dev/null || echo "  Release not found or incomplete"
echo

echo "3. Cattle-System Namespace:"
kubectl get ns cattle-system -o yaml 2>/dev/null || echo "  Namespace does not exist"
echo

echo "4. All Resources in cattle-system:"
kubectl get all -n cattle-system -o wide
echo

echo "5. Rancher Deployments:"
kubectl get deployments -n cattle-system -o wide
echo

echo "6. Rancher Pods:"
kubectl get pods -n cattle-system -o wide
echo

echo "7. Rancher Services:"
kubectl get svc -n cattle-system
echo

echo "8. Rancher Ingresses:"
kubectl get ingress -n cattle-system
echo

echo "9. Events in cattle-system:"
kubectl get events -n cattle-system --sort-by='.lastTimestamp' | tail -30
echo

echo "10. Helm Secrets (releases):"
kubectl get secrets -n cattle-system -l owner=helm
echo

echo "11. Check if Rancher CRDs exist:"
kubectl get crd | grep cattle || echo "  No cattle CRDs found"
echo

echo "12. Check validating webhooks (might block install):"
kubectl get validatingwebhookconfiguration
echo

echo "13. Check mutating webhooks (might block install):"
kubectl get mutatingwebhookconfiguration
echo

echo "14. Describe Rancher deployment (if exists):"
kubectl describe deployment rancher -n cattle-system 2>/dev/null || echo "  Deployment not found"
echo

echo "15. Rancher Pod Details (if any exist):"
for pod in $(kubectl get pods -n cattle-system -l app=rancher -o name 2>/dev/null); do
    echo "--- $pod ---"
    kubectl describe $pod -n cattle-system
    echo
    kubectl logs $pod -n cattle-system --all-containers=true --tail=50 2>/dev/null || echo "  No logs yet"
    echo
done

echo "16. Check cert-manager is still healthy:"
kubectl get pods -n cert-manager
echo

echo "17. Check if there are pending PVCs:"
kubectl get pvc -n cattle-system
echo

echo "=== Common Helm Issues ==="
echo
echo "If helm install is stuck:"
echo "  1. Ctrl+C the helm install"
echo "  2. Check: helm list -n cattle-system -a"
echo "  3. If status is 'pending-install', run: helm uninstall rancher -n cattle-system"
echo "  4. If deployment exists but pods don't: kubectl describe deployment rancher -n cattle-system"
echo "  5. Check for webhook issues: kubectl get validatingwebhookconfiguration"
echo
echo "To force uninstall:"
echo "  helm uninstall rancher -n cattle-system --no-hooks"
echo "  kubectl delete namespace cattle-system --force --grace-period=0"
