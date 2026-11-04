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
#
# Stand up the tier-1 kind cluster. Delegates to the e2e cluster setup so the
# benchmark runs against the exact node topology the conformance suite uses.

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"

echo "Creating kind cluster for performance benchmarks..."
"${ROOT}/test/e2e/00-setup.sh"
