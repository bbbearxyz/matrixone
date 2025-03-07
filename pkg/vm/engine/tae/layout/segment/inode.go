// Copyright 2021 Matrix Origin
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

package segment

import "sync"

type StateType uint8

const (
	RESIDENT StateType = iota
	REMOVE
)

type Inode struct {
	inode      uint64
	algo       uint8
	size       uint64
	originSize uint64
	mutex      sync.RWMutex
	extents    []Extent
	logExtents Extent
	state      StateType
}
