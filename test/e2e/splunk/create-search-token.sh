#!/bin/bash
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

# SPLUNK_SEARCH_TOKEN for the Results plugin.
#
# Production uses a Splunk authentication token (JWT, Bearer). This e2e cluster
# cannot mint one: SPLUNK_LICENSE_URI=Free has no license feature "Auth"
# (GET /services/admin/token-auth returns 402; POST /services/authorization/tokens
# closes the connection). Admin Basic (admin:password) is the documented search
# REST fallback on Free. The plugin still sends Bearer when the token is a JWT
# (eyJ*), so licensed Splunk is unchanged.
set -euo pipefail
PASS=$(kubectl get secret splunk-secrets -n logging -o jsonpath='{.data.password}' | base64 -d | tr -d '\n')

ready=false
ok=0
for _ in $(seq 1 90); do
  if kubectl exec -n logging sts/splunk -- curl -sk --connect-timeout 5 --max-time 15 \
       -u "admin:${PASS}" "https://127.0.0.1:8089/services/server/info" >/dev/null 2>&1; then
    ok=$((ok + 1))
    if [ "${ok}" -ge 3 ]; then
      ready=true
      break
    fi
  else
    ok=0
  fi
  sleep 5
done
if [ "${ready}" != "true" ]; then
  echo "Splunk management API did not become ready" >&2
  exit 1
fi

kubectl create secret generic splunk-search-token -n tekton-pipelines \
  --from-literal=SPLUNK_SEARCH_TOKEN="admin:${PASS}" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl set env deployment/tekton-results-api -n tekton-pipelines \
  --from=secret/splunk-search-token
