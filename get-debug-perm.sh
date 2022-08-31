#!/bin/bash

function getToken() {
    # Create a secret to hold a token for the "tekton-results-debug" service account
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: default-token
  namespace: tekton-pipelines
  annotations:
    kubernetes.io/service-account.name: tekton-results-debug
type: kubernetes.io/service-account-token
EOF

    # Wait for the token controller to populate the secret with a token:
    while ! kubectl describe secret default-token -n tekton-pipelines | grep -E '^token' >/dev/null; do
    echo "waiting for token..." >&2
    sleep 1
    done

    # Get the token value
    $(kubectl get secret default-token -n tekton-pipelines -o jsonpath='{.data.token}' | base64 --decode > /tmp/token)

    TOKEN=$(cat /tmp/token)
}

export KO_DOCKER_REPO="kind.local"

kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tekton-results-readonly
  namespace: tekton-pipelines
rules:
  - apiGroups: ["results.tekton.dev"]
    resources: ["results", "records"]
    verbs: ["get", "list"]
EOF

kubectl create sa tekton-results-debug -n tekton-pipelines || true

kubectl create clusterrolebinding tekton-results-debug \
--clusterrole=tekton-results-readonly \
--serviceaccount=tekton-pipelines:tekton-results-debug || true

kubectl get secrets tekton-results-tls -n tekton-pipelines --template='{{index .data 
"tls.crt"}}' | base64 -d > /tmp/results.crt || true

GRPC_DEFAULT_SSL_ROOTS_FILE_PATH=/tmp/results.crt

getToken

export GRPC_DEFAULT_SSL_ROOTS_FILE_PATH
export TOKEN
