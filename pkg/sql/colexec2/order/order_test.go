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

package order

import (
	"bytes"
	"strconv"
	"testing"

	batch "github.com/matrixorigin/matrixone/pkg/container/batch2"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/container/vector"
	"github.com/matrixorigin/matrixone/pkg/encoding"
	"github.com/matrixorigin/matrixone/pkg/vm/mheap"
	"github.com/matrixorigin/matrixone/pkg/vm/mmu/guest"
	"github.com/matrixorigin/matrixone/pkg/vm/mmu/host"
	process "github.com/matrixorigin/matrixone/pkg/vm/process2"
	"github.com/stretchr/testify/require"
)

const (
	Rows          = 10     // default rows
	BenchmarkRows = 100000 // default rows for benchmark
)

// add unit tests for cases
type orderTestCase struct {
	arg   *Argument
	types []types.Type
	proc  *process.Process
}

var (
	tcs []orderTestCase
)

func init() {
	hm := host.New(1 << 30)
	gm := guest.New(1<<30, hm)
	tcs = []orderTestCase{
		newTestCase(mheap.New(gm), []types.Type{{Oid: types.T_int8}}, []Field{{Pos: 0, Type: 0}}),
		newTestCase(mheap.New(gm), []types.Type{{Oid: types.T_int8}}, []Field{{Pos: 0, Type: 2}}),
		newTestCase(mheap.New(gm), []types.Type{{Oid: types.T_int8}, {Oid: types.T_int64}}, []Field{{Pos: 0, Type: 0}, {Pos: 1, Type: 0}}),
		newTestCase(mheap.New(gm), []types.Type{{Oid: types.T_int8}, {Oid: types.T_int64}}, []Field{{Pos: 0, Type: 2}, {Pos: 1, Type: 2}}),
	}
}

func TestString(t *testing.T) {
	buf := new(bytes.Buffer)
	for _, tc := range tcs {
		String(tc.arg, buf)
	}
}

func TestPrepare(t *testing.T) {
	for _, tc := range tcs {
		Prepare(tc.proc, tc.arg)
	}
}

func TestOrder(t *testing.T) {
	for _, tc := range tcs {
		Prepare(tc.proc, tc.arg)
		tc.proc.Reg.InputBatch = newBatch(t, tc.types, tc.proc, Rows)
		Call(tc.proc, tc.arg)
		if tc.proc.Reg.InputBatch != nil {
			batch.Clean(tc.proc.Reg.InputBatch, tc.proc.Mp)
		}
		tc.proc.Reg.InputBatch = newBatch(t, tc.types, tc.proc, Rows)
		Call(tc.proc, tc.arg)
		if tc.proc.Reg.InputBatch != nil {
			batch.Clean(tc.proc.Reg.InputBatch, tc.proc.Mp)
		}
		tc.proc.Reg.InputBatch = &batch.Batch{}
		Call(tc.proc, tc.arg)
		tc.proc.Reg.InputBatch = nil
		Call(tc.proc, tc.arg)
		require.Equal(t, mheap.Size(tc.proc.Mp), int64(0))
	}
}

func BenchmarkOrder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		hm := host.New(1 << 30)
		gm := guest.New(1<<30, hm)
		tcs = []orderTestCase{
			newTestCase(mheap.New(gm), []types.Type{{Oid: types.T_int8}}, []Field{{Pos: 0, Type: 0}}),
			newTestCase(mheap.New(gm), []types.Type{{Oid: types.T_int8}}, []Field{{Pos: 0, Type: 2}}),
			newTestCase(mheap.New(gm), []types.Type{{Oid: types.T_int8}, {Oid: types.T_int64}}, []Field{{Pos: 0, Type: 0}, {Pos: 1, Type: 0}}),
			newTestCase(mheap.New(gm), []types.Type{{Oid: types.T_int8}, {Oid: types.T_int64}}, []Field{{Pos: 0, Type: 2}, {Pos: 1, Type: 2}}),
		}
		t := new(testing.T)
		for _, tc := range tcs {
			Prepare(tc.proc, tc.arg)
			tc.proc.Reg.InputBatch = newBatch(t, tc.types, tc.proc, BenchmarkRows)
			Call(tc.proc, tc.arg)
			if tc.proc.Reg.InputBatch != nil {
				batch.Clean(tc.proc.Reg.InputBatch, tc.proc.Mp)
			}
			tc.proc.Reg.InputBatch = newBatch(t, tc.types, tc.proc, BenchmarkRows)
			Call(tc.proc, tc.arg)
			if tc.proc.Reg.InputBatch != nil {
				batch.Clean(tc.proc.Reg.InputBatch, tc.proc.Mp)
			}
			tc.proc.Reg.InputBatch = &batch.Batch{}
			Call(tc.proc, tc.arg)
			tc.proc.Reg.InputBatch = nil
			Call(tc.proc, tc.arg)
		}
	}
}

func newTestCase(m *mheap.Mheap, ts []types.Type, fs []Field) orderTestCase {
	return orderTestCase{
		types: ts,
		proc:  process.New(m),
		arg: &Argument{
			Fs: fs,
		},
	}
}

// create a new block based on the type information
func newBatch(t *testing.T, ts []types.Type, proc *process.Process, rows int64) *batch.Batch {
	bat := batch.New(len(ts))
	bat.InitZsOne(int(rows))
	for i := range bat.Vecs {
		vec := vector.New(ts[i])
		switch vec.Typ.Oid {
		case types.T_int8:
			data, err := mheap.Alloc(proc.Mp, rows*1)
			require.NoError(t, err)
			vec.Data = data
			vs := encoding.DecodeInt8Slice(vec.Data)[:rows]
			for i := range vs {
				vs[i] = int8(i)
			}
			vec.Col = vs
		case types.T_int64:
			data, err := mheap.Alloc(proc.Mp, rows*8)
			require.NoError(t, err)
			vec.Data = data
			vs := encoding.DecodeInt64Slice(vec.Data)[:rows]
			for i := range vs {
				vs[i] = int64(i)
			}
			vec.Col = vs
		case types.T_float64:
			data, err := mheap.Alloc(proc.Mp, rows*8)
			require.NoError(t, err)
			vec.Data = data
			vs := encoding.DecodeFloat64Slice(vec.Data)[:rows]
			for i := range vs {
				vs[i] = float64(i)
			}
			vec.Col = vs
		case types.T_date:
			data, err := mheap.Alloc(proc.Mp, rows*4)
			require.NoError(t, err)
			vec.Data = data
			vs := encoding.DecodeDateSlice(vec.Data)[:rows]
			for i := range vs {
				vs[i] = types.Date(i)
			}
			vec.Col = vs
		case types.T_char, types.T_varchar:
			size := 0
			vs := make([][]byte, rows)
			for i := range vs {
				vs[i] = []byte(strconv.Itoa(i))
				size += len(vs[i])
			}
			data, err := mheap.Alloc(proc.Mp, int64(size))
			require.NoError(t, err)
			data = data[:0]
			col := new(types.Bytes)
			o := uint32(0)
			for _, v := range vs {
				data = append(data, v...)
				col.Offsets = append(col.Offsets, o)
				o += uint32(len(v))
				col.Lengths = append(col.Lengths, uint32(len(v)))
			}
			col.Data = data
			vec.Col = col
			vec.Data = data
		}
		bat.Vecs[i] = vec
	}
	return bat
}
