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

package decimal64s

import (
	"github.com/matrixorigin/matrixone/pkg/container/nulls"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/container/vector"
	process "github.com/matrixorigin/matrixone/pkg/vm/process2"
)

func New() *compare {
	return &compare{
		xs: make([][]types.Decimal64, 2),
		ns: make([]*nulls.Nulls, 2),
		vs: make([]*vector.Vector, 2),
	}
}

func (c *compare) Vector() *vector.Vector {
	return c.vs[0]
}

func (c *compare) Set(idx int, v *vector.Vector) {
	c.vs[idx] = v
	c.ns[idx] = v.Nsp
	c.xs[idx] = v.Col.([]types.Decimal64)
}

// Compare method for decimal needs to know the decimal's scale, so we need to fill in the c.vs field before using this function
func (c *compare) Compare(veci, vecj int, vi, vj int64) int {
	return int(types.CompareDecimal64Decimal64(c.xs[veci][vi], c.xs[vecj][vj], c.vs[0].Typ.Scale, c.vs[1].Typ.Scale))
}

func (c *compare) Copy(vecSrc, vecDst int, src, dst int64, _ *process.Process) error {
	if nulls.Any(c.ns[vecSrc]) && nulls.Contains(c.ns[vecSrc], (uint64(src))) {
		nulls.Add(c.ns[vecDst], (uint64(dst)))
	} else {
		nulls.Del(c.ns[vecDst], (uint64(dst)))
		c.xs[vecDst][dst] = c.xs[vecSrc][src]
	}
	return nil
}
