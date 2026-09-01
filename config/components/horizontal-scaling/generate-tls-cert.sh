#!/usr/bin/env bash
# Copyright 2026 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

# This script generates a TLS certificate for the Tekton Results API service.
# By default it includes both the regular ClusterIP service and the headless
# service in the SAN, which is what horizontal scaling deployments need since
# the watcher connects to the API via the headless service for DNS-based load
# balancing. Callers that only need the regular service name (e.g. a
# single-replica deployment) can opt out by setting EXTRA_SANS="".

NAMESPACE="${NAMESPACE:-tekton-pipelines}"
SECRET_NAME="${SECRET_NAME:-tekton-results-tls}"
CERT_DAYS="${CERT_DAYS:-365}"
OUTPUT_DIR="${OUTPUT_DIR:-./}"
CERT_FILE_NAME="${CERT_FILE_NAME:-cert.pem}"
KEY_FILE_NAME="${KEY_FILE_NAME:-key.pem}"
SSL_INCLUDE_LOCALHOST="${SSL_INCLUDE_LOCALHOST:-false}"
# NOTE: uses "${VAR-default}" (no colon) rather than "${VAR:-default}" so that
# an explicitly empty EXTRA_SANS="" disables the headless SAN instead of
# falling back to the default.
EXTRA_SANS="${EXTRA_SANS-DNS:tekton-results-api-service-headless.${NAMESPACE}.svc.cluster.local}"

CERT_PATH="${OUTPUT_DIR}/${CERT_FILE_NAME}"
KEY_PATH="${OUTPUT_DIR}/${KEY_FILE_NAME}"

echo "Generating TLS certificate..."
echo "  Namespace: ${NAMESPACE}"
echo "  Secret: ${SECRET_NAME}"
echo "  Valid for: ${CERT_DAYS} days"

# Subject Alternative Names always include the regular ClusterIP service,
# plus any caller-supplied extras (e.g. the headless service, localhost).
SANS="DNS:tekton-results-api-service.${NAMESPACE}.svc.cluster.local"
if [ -n "${EXTRA_SANS}" ]; then
    SANS="${SANS},${EXTRA_SANS}"
fi
if [ "${SSL_INCLUDE_LOCALHOST}" = "true" ]; then
    SANS="${SANS},DNS:localhost"
fi

# Common Name is the primary service
CN="tekton-results-api-service.${NAMESPACE}.svc.cluster.local"

# Try modern OpenSSL syntax first (with -addext flag)
set +e
if ! openssl req -x509 \
    -newkey rsa:4096 \
    -keyout "${KEY_PATH}" \
    -out "${CERT_PATH}" \
    -days "${CERT_DAYS}" \
    -nodes \
    -subj "/CN=${CN}" \
    -addext "subjectAltName = ${SANS}" \
    2>/dev/null; then
    echo "Modern OpenSSL syntax failed, trying legacy LibreSSL syntax..."

    # Fallback for older LibreSSL versions (e.g., macOS Big Sur)
    if ! openssl req -x509 \
        -verbose \
        -config <(cat /etc/ssl/openssl.cnf <(printf "[SAN]\nsubjectAltName = %s" "${SANS}")) \
        -extensions SAN \
        -newkey rsa:4096 \
        -keyout "${KEY_PATH}" \
        -out "${CERT_PATH}" \
        -days "${CERT_DAYS}" \
        -nodes \
        -subj "/CN=${CN}"; then
        echo "ERROR: Failed to generate TLS certificate" >&2
        exit 1
    fi
fi
set -e

echo "Certificate generated successfully:"
echo "  - ${CERT_PATH}"
echo "  - ${KEY_PATH}"

# Verify the SAN entries
echo ""
echo "Subject Alternative Names in certificate:"
openssl x509 -in "${CERT_PATH}" -noout -text | grep -A1 "Subject Alternative Name"

# Create or update the Kubernetes secret
echo ""
echo "Creating/updating Kubernetes secret..."
if kubectl create secret tls -n "${NAMESPACE}" "${SECRET_NAME}" \
    --cert="${CERT_PATH}" \
    --key="${KEY_PATH}" \
    --dry-run=client -o yaml | kubectl apply -f -; then
    echo "Secret ${SECRET_NAME} created/updated successfully in namespace ${NAMESPACE}"
else
    echo "ERROR: Failed to create secret. You can manually create it with:" >&2
    echo "  kubectl create secret tls -n ${NAMESPACE} ${SECRET_NAME} --cert=${CERT_PATH} --key=${KEY_PATH}" >&2
    exit 1
fi

echo ""
echo "TLS certificate setup complete!"
