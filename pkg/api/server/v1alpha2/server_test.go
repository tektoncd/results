// Copyright 2020 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	cw "github.com/jonboulle/clockwork"
)

var (
	// Used for deterministically increasing UUID generation.
	lastID                  = uint32(0)
	fakeClock *cw.FakeClock = cw.NewFakeClock()
)

func TestMain(m *testing.M) {
	uid = func() string {
		return fmt.Sprint(atomic.AddUint32(&lastID, 1))
	}
	clock = fakeClock
	os.Exit(m.Run())
}
