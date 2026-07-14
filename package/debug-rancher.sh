#!/bin/bash
# Debug script for Rancher installation issues (helm-controller mode)

echo "=== Rancher Debug Information ==="
echo

echo "1. HelmChart CR Status:"
kubectl get helmchart rancher -n kube-system -o yaml 2>/dev/null || \
    echo "  No HelmChart CR found (may be using helm CLI mode)"
echo

echo "2. Helm-controller Job Status:"
kubectl get job helm-install-rancher -n kube-system -o wide 2>/dev/null || \
    echo "  No helm-install-rancher job found"
echo

echo "3. Helm-controller Job Logs:"
kubectl logs -n kube-system -l job-name=helm-install-rancher --tail=50 2>/dev/null || \
    echo "  No job logs available"
echo

echo "4. Cattle-System Namespace:"
kubectl get ns cattle-system -o yaml 2>/dev/null || echo "  Namespace does not exist"
echo

echo "5. All Resources in cattle-system:"
kubectl get all -n cattle-system -o wide
echo

echo "6. Rancher Deployments:"
kubectl get deployments -n cattle-system -o wide
echo

echo "7. Rancher Pods:"
kubectl get pods -n cattle-system -o wide
echo

echo "8. Rancher Services:"
kubectl get svc -n cattle-system
echo

echo "9. Rancher Ingresses:"
kubectl get ingress -n cattle-system
echo

echo "10. Events in cattle-system:"
kubectl get events -n cattle-system --sort-by='.lastTimestamp' | tail -30
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

echo "17. HelmChart CRs in kube-system (all):"
kubectl get helmchart -n kube-system
echo

echo "18. Check if there are pending PVCs:"
kubectl get pvc -n cattle-system
echo

echo "=== Common Issues ==="
echo
echo "If helm-controller job is stuck or failed:"
echo "  kubectl logs -n kube-system -l job-name=helm-install-rancher"
echo "  kubectl delete helmchart rancher -n kube-system  # triggers re-install"
echo
echo "If namespace is stuck terminating:"
echo "  kubectl delete namespace cattle-system --force --grace-period=0"
echo
