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

package left

import (
	"bytes"
	"unsafe"

	batch "github.com/matrixorigin/matrixone/pkg/container/batch2"
	"github.com/matrixorigin/matrixone/pkg/container/hashtable"
	"github.com/matrixorigin/matrixone/pkg/container/nulls"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/container/vector"
	process "github.com/matrixorigin/matrixone/pkg/vm/process2"
)

func init() {
	OneInt64s = make([]int64, UnitLimit)
	for i := range OneInt64s {
		OneInt64s[i] = 1
	}
}

func String(_ interface{}, buf *bytes.Buffer) {
	buf.WriteString(" ⟕ ")
}

func Prepare(proc *process.Process, arg interface{}) error {
	ap := arg.(*Argument)
	ap.ctr = new(Container)
	ap.ctr.keys = make([][]byte, UnitLimit)
	ap.ctr.values = make([]uint64, UnitLimit)
	ap.ctr.zValues = make([]int64, UnitLimit)
	ap.ctr.inserted = make([]uint8, UnitLimit)
	ap.ctr.zInserted = make([]uint8, UnitLimit)
	ap.ctr.strHashStates = make([][3]uint64, UnitLimit)
	ap.ctr.strHashMap = &hashtable.StringHashMap{}
	ap.ctr.strHashMap.Init()
	mp := make(map[int32]int)
	for i, cond := range ap.Conditions[0] { // aligning the precision of decimal
		mp[ap.Conditions[1][i].Pos]++
		switch cond.Typ.Oid {
		case types.T_decimal64:
			typ := ap.Conditions[1][i]
			if typ.Scale > cond.Typ.Scale {
				cond.Scale = typ.Scale - cond.Typ.Scale
			} else if typ.Scale < cond.Typ.Scale {
				ap.Conditions[1][i].Scale = cond.Typ.Scale - typ.Scale
			}
		case types.T_decimal128:
			typ := ap.Conditions[1][i]
			if typ.Scale > cond.Typ.Scale {
				cond.Scale = typ.Scale - cond.Typ.Scale
			} else if typ.Scale < cond.Typ.Scale {
				ap.Conditions[1][i].Scale = cond.Typ.Scale - typ.Scale
			}
		}
	}
	{
		flg := false
		for _, rp := range ap.Result {
			if rp.Rel == 1 {
				ap.ctr.poses = append(ap.ctr.poses, rp.Pos)
				if _, ok := mp[rp.Pos]; ok {
					continue
				}
				flg = true
			}
		}
		ap.ctr.flg = flg
	}
	ap.ctr.decimal64Slice = make([]types.Decimal64, UnitLimit)
	ap.ctr.decimal128Slice = make([]types.Decimal128, UnitLimit)
	return nil
}

func Call(proc *process.Process, arg interface{}) (bool, error) {
	ap := arg.(*Argument)
	ctr := ap.ctr
	for {
		switch ctr.state {
		case Build:
			if err := ctr.build(ap, proc); err != nil {
				ctr.state = End
				return true, err
			}
			ctr.state = Probe
		case Probe:
			bat := <-proc.Reg.MergeReceivers[0].Ch
			if bat == nil {
				ctr.state = End
				batch.Clean(ctr.bat, proc.Mp)
				continue
			}
			if len(bat.Zs) == 0 {
				continue
			}
			if err := ctr.probe(bat, ap, proc); err != nil {
				ctr.state = End
				proc.Reg.InputBatch = nil
				return true, err
			}
			return false, nil
		default:
			proc.Reg.InputBatch = nil
			return true, nil
		}
	}
}

func (ctr *Container) build(ap *Argument, proc *process.Process) error {
	if ap.IsPreBuild {
		bat := <-proc.Reg.MergeReceivers[1].Ch
		ctr.bat = bat
		ctr.strHashMap = bat.Ht.(*hashtable.StringHashMap)
		return nil
	}
	if ctr.flg {
		var err error

		for {
			bat := <-proc.Reg.MergeReceivers[1].Ch
			if bat == nil {
				break
			}
			if len(bat.Zs) == 0 {
				continue
			}
			if ctr.bat == nil {
				ctr.bat = batch.New(len(bat.Vecs))
				for i, vec := range bat.Vecs {
					ctr.bat.Vecs[i] = vector.New(vec.Typ)
				}
			}
			if ctr.bat, err = ctr.bat.Append(proc.Mp, bat); err != nil {
				batch.Clean(bat, proc.Mp)
				batch.Clean(ctr.bat, proc.Mp)
				return err
			}
			batch.Clean(bat, proc.Mp)
		}
		count := len(ctr.bat.Zs)
		for i := 0; i < count; i += UnitLimit {
			n := count - i
			if n > UnitLimit {
				n = UnitLimit
			}
			copy(ctr.zValues[:n], OneInt64s[:n])
			for _, cond := range ap.Conditions[1] {
				vec := ctr.bat.Vecs[cond.Pos]
				switch typLen := vec.Typ.Oid.FixedLength(); typLen {
				case 1:
					fillGroupStr[uint8](ctr, vec, n, 1, i)
				case 2:
					fillGroupStr[uint16](ctr, vec, n, 2, i)
				case 4:
					fillGroupStr[uint32](ctr, vec, n, 4, i)
				case 8:
					fillGroupStr[uint64](ctr, vec, n, 8, i)
				case -8:
					if cond.Scale > 0 {
						fillGroupStrWithDecimal64(ctr, vec, n, i, cond.Scale)
					} else {
						fillGroupStr[uint64](ctr, vec, n, 8, i)
					}
				case -16:
					if cond.Scale > 0 {
						fillGroupStrWithDecimal128(ctr, vec, n, i, cond.Scale)
					} else {
						fillGroupStr[types.Decimal128](ctr, vec, n, 16, i)
					}
				default:
					vs := vec.Col.(*types.Bytes)
					if !nulls.Any(vec.Nsp) {
						for k := 0; k < n; k++ {
							ctr.keys[k] = append(ctr.keys[k], vs.Get(int64(i+k))...)
						}
					} else {
						for k := 0; k < n; k++ {
							if vec.Nsp.Np.Contains(uint64(i + k)) {
								ctr.zValues[i] = 0
							} else {
								ctr.keys[k] = append(ctr.keys[k], vs.Get(int64(i+k))...)
							}
						}
					}
				}
			}
			for k := 0; k < n; k++ {
				if l := len(ctr.keys[k]); l < 16 {
					ctr.keys[k] = append(ctr.keys[k], hashtable.StrKeyPadding[l:]...)
				}
			}
			ctr.strHashMap.InsertStringBatchWithRing(ctr.zValues, ctr.strHashStates, ctr.keys[:n], ctr.values)
			for k, v := range ctr.values[:n] {
				if ctr.zValues[k] == 0 {
					continue
				}
				if v > ctr.rows {
					ctr.sels = append(ctr.sels, make([]int64, 0, 8))
				}
				ai := int64(v) - 1
				ctr.sels[ai] = append(ctr.sels[ai], int64(i+k))
			}
			for k := 0; k < n; k++ {
				ctr.keys[k] = ctr.keys[k][:0]
			}
		}
		return nil
	}
	for {
		bat := <-proc.Reg.MergeReceivers[1].Ch
		if bat == nil {
			return nil
		}
		if len(bat.Zs) == 0 {
			continue
		}
		if ctr.bat == nil {
			ctr.bat = batch.New(len(bat.Vecs))
			for _, pos := range ctr.poses {
				ctr.bat.Vecs[pos] = vector.New(bat.Vecs[pos].Typ)
			}
		}
		count := len(bat.Zs)
		for i := 0; i < count; i += UnitLimit {
			n := count - i
			if n > UnitLimit {
				n = UnitLimit
			}
			copy(ctr.zValues[:n], OneInt64s[:n])
			for _, cond := range ap.Conditions[1] {
				vec := bat.Vecs[cond.Pos]
				switch typLen := vec.Typ.Oid.FixedLength(); typLen {
				case 1:
					fillGroupStr[uint8](ctr, vec, n, 1, i)
				case 2:
					fillGroupStr[uint16](ctr, vec, n, 2, i)
				case 4:
					fillGroupStr[uint32](ctr, vec, n, 4, i)
				case 8:
					fillGroupStr[uint64](ctr, vec, n, 8, i)
				case -8:
					if cond.Scale > 0 {
						fillGroupStrWithDecimal64(ctr, vec, n, i, cond.Scale)
					} else {
						fillGroupStr[uint64](ctr, vec, n, 8, i)
					}
				case -16:
					if cond.Scale > 0 {
						fillGroupStrWithDecimal128(ctr, vec, n, i, cond.Scale)
					} else {
						fillGroupStr[types.Decimal128](ctr, vec, n, 16, i)
					}
				default:
					vs := vec.Col.(*types.Bytes)
					if !nulls.Any(vec.Nsp) {
						for k := 0; k < n; k++ {
							ctr.keys[k] = append(ctr.keys[k], vs.Get(int64(i+k))...)
						}
					} else {
						for k := 0; k < n; k++ {
							if vec.Nsp.Np.Contains(uint64(i + k)) {
								ctr.zValues[i] = 0
							} else {
								ctr.keys[k] = append(ctr.keys[k], vs.Get(int64(i+k))...)
							}
						}
					}
				}
			}
			for k := 0; k < n; k++ {
				if l := len(ctr.keys[k]); l < 16 {
					ctr.keys[k] = append(ctr.keys[k], hashtable.StrKeyPadding[l:]...)
				}
			}
			ctr.strHashMap.InsertStringBatchWithRing(ctr.zValues, ctr.strHashStates, ctr.keys[:n], ctr.values)
			cnt := 0
			copy(ctr.inserted[:n], ctr.zInserted[:n])
			for k, v := range ctr.values[:n] {
				if ctr.zValues[k] == 0 {
					continue
				}
				if v > ctr.rows {
					cnt++
					ctr.rows++
					ctr.inserted[k] = 1
					ctr.bat.Zs = append(ctr.bat.Zs, 0)
				}
				ai := int64(v) - 1
				ctr.bat.Zs[ai] += bat.Zs[i+k]
			}
			if cnt > 0 {
				for _, pos := range ctr.poses {
					if err := vector.UnionBatch(ctr.bat.Vecs[pos], bat.Vecs[pos], int64(i), cnt, ctr.inserted[:n], proc.Mp); err != nil {
						batch.Clean(bat, proc.Mp)
						batch.Clean(ctr.bat, proc.Mp)
						return err
					}

				}
			}
			for k := 0; k < n; k++ {
				ctr.keys[k] = ctr.keys[k][:0]
			}
			batch.Clean(bat, proc.Mp)
		}
	}
}

func (ctr *Container) probe(bat *batch.Batch, ap *Argument, proc *process.Process) error {
	defer batch.Clean(bat, proc.Mp)
	rbat := batch.New(len(ap.Result))
	for i, rp := range ap.Result {
		if rp.Rel == 0 {
			rbat.Vecs[i] = vector.New(bat.Vecs[rp.Pos].Typ)
		} else {
			rbat.Vecs[i] = vector.New(ctr.bat.Vecs[rp.Pos].Typ)
		}
	}
	count := len(bat.Zs)
	for i := 0; i < count; i += UnitLimit {
		n := count - i
		if n > UnitLimit {
			n = UnitLimit
		}
		copy(ctr.zValues[:n], OneInt64s[:n])
		for _, cond := range ap.Conditions[0] {
			vec := bat.Vecs[cond.Pos]
			switch typLen := vec.Typ.Oid.FixedLength(); typLen {
			case 1:
				fillGroupStr[uint8](ctr, vec, n, 1, i)
			case 2:
				fillGroupStr[uint16](ctr, vec, n, 2, i)
			case 4:
				fillGroupStr[uint32](ctr, vec, n, 4, i)
			case 8:
				fillGroupStr[uint64](ctr, vec, n, 8, i)
			case -8:
				if cond.Scale > 0 {
					fillGroupStrWithDecimal64(ctr, vec, n, i, cond.Scale)
				} else {
					fillGroupStr[uint64](ctr, vec, n, 8, i)
				}
			case -16:
				if cond.Scale > 0 {
					fillGroupStrWithDecimal128(ctr, vec, n, i, cond.Scale)
				} else {
					fillGroupStr[types.Decimal128](ctr, vec, n, 16, i)
				}
			default:
				vs := vec.Col.(*types.Bytes)
				if !nulls.Any(vec.Nsp) {
					for k := 0; k < n; k++ {
						ctr.keys[k] = append(ctr.keys[k], vs.Get(int64(i+k))...)
					}
				} else {
					for k := 0; k < n; k++ {
						if vec.Nsp.Np.Contains(uint64(i + k)) {
							ctr.zValues[i] = 0
						} else {
							ctr.keys[k] = append(ctr.keys[k], vs.Get(int64(i+k))...)
						}
					}
				}
			}
		}
		for k := 0; k < n; k++ {
			if l := len(ctr.keys[k]); l < 16 {
				ctr.keys[k] = append(ctr.keys[k], hashtable.StrKeyPadding[l:]...)
			}
		}
		ctr.strHashMap.FindStringBatch(ctr.strHashStates, ctr.keys[:n], ctr.values)
		for k := 0; k < n; k++ {
			ctr.keys[k] = ctr.keys[k][:0]
		}
		for k := 0; k < n; k++ {
			if ctr.zValues[k] == 0 || ctr.values[k] == 0 {
				for j, rp := range ap.Result {
					if rp.Rel == 0 {
						if err := vector.UnionOne(rbat.Vecs[j], bat.Vecs[rp.Pos], int64(i+k), proc.Mp); err != nil {
							batch.Clean(rbat, proc.Mp)
							return err
						}
					} else {
						if err := vector.UnionNull(rbat.Vecs[j], ctr.bat.Vecs[rp.Pos], proc.Mp); err != nil {
							batch.Clean(rbat, proc.Mp)
							return err
						}
					}
				}
				rbat.Zs = append(rbat.Zs, bat.Zs[i+k])
				continue
			}
			if ctr.flg {
				sels := ctr.sels[ctr.values[k]-1]
				for _, sel := range sels {
					for j, rp := range ap.Result {
						if rp.Rel == 0 {
							if err := vector.UnionOne(rbat.Vecs[j], bat.Vecs[rp.Pos], int64(i+k), proc.Mp); err != nil {
								batch.Clean(rbat, proc.Mp)
								return err
							}
						} else {
							if err := vector.UnionOne(rbat.Vecs[j], ctr.bat.Vecs[rp.Pos], sel, proc.Mp); err != nil {
								batch.Clean(rbat, proc.Mp)
								return err
							}
						}
					}
					rbat.Zs = append(rbat.Zs, ctr.bat.Zs[sel])
				}
			} else {
				sel := int64(ctr.values[k] - 1)
				for j, rp := range ap.Result {
					if rp.Rel == 0 {
						if err := vector.UnionOne(rbat.Vecs[j], bat.Vecs[rp.Pos], int64(i+k), proc.Mp); err != nil {
							batch.Clean(rbat, proc.Mp)
							return err
						}
					} else {
						if err := vector.UnionOne(rbat.Vecs[j], ctr.bat.Vecs[rp.Pos], sel, proc.Mp); err != nil {
							batch.Clean(rbat, proc.Mp)
							return err
						}
					}
				}
				rbat.Zs = append(rbat.Zs, ctr.bat.Zs[sel])
			}
		}
	}
	proc.Reg.InputBatch = rbat
	return nil
}

func fillGroupStr[T any](ctr *Container, vec *vector.Vector, n int, sz int, start int) {
	vs := vector.DecodeFixedCol[T](vec, sz)
	data := unsafe.Slice((*byte)(unsafe.Pointer(&vs[0])), cap(vs)*sz)[:len(vs)*sz]
	if !nulls.Any(vec.Nsp) {
		for i := 0; i < n; i++ {
			ctr.keys[i] = append(ctr.keys[i], data[(i+start)*sz:(i+start+1)*sz]...)
		}
	} else {
		for i := 0; i < n; i++ {
			if vec.Nsp.Np.Contains(uint64(i + start)) {
				ctr.zValues[i] = 0
			} else {
				ctr.keys[i] = append(ctr.keys[i], data[(i+start)*sz:(i+start+1)*sz]...)
			}
		}
	}
}

func fillGroupStrWithDecimal64(ctr *Container, vec *vector.Vector, n int, start int, scale int32) {
	src := vector.DecodeFixedCol[types.Decimal64](vec, 8)
	vs := types.AlignDecimal64UsingScaleDiffBatch(src[start:start+n], ctr.decimal64Slice[:n], scale)
	data := unsafe.Slice((*byte)(unsafe.Pointer(&vs[0])), cap(vs)*8)[:len(vs)*8]
	if !nulls.Any(vec.Nsp) {
		for i := 0; i < n; i++ {
			ctr.keys[i] = append(ctr.keys[i], data[(i)*8:(i+1)*8]...)
		}
	} else {
		for i := 0; i < n; i++ {
			if vec.Nsp.Np.Contains(uint64(i + start)) {
				ctr.zValues[i] = 0
			} else {
				ctr.keys[i] = append(ctr.keys[i], data[(i)*8:(i+1)*8]...)
			}
		}
	}
}

func fillGroupStrWithDecimal128(ctr *Container, vec *vector.Vector, n int, start int, scale int32) {
	src := vector.DecodeFixedCol[types.Decimal128](vec, 16)
	vs := ctr.decimal128Slice[:n]
	types.AlignDecimal128UsingScaleDiffBatch(src[start:start+n], vs, scale)
	data := unsafe.Slice((*byte)(unsafe.Pointer(&vs[0])), cap(vs)*16)[:len(vs)*16]
	if !nulls.Any(vec.Nsp) {
		for i := 0; i < n; i++ {
			ctr.keys[i] = append(ctr.keys[i], data[(i)*16:(i+1)*16]...)
		}
	} else {
		for i := 0; i < n; i++ {
			if vec.Nsp.Np.Contains(uint64(i + start)) {
				ctr.zValues[i] = 0
			} else {
				ctr.keys[i] = append(ctr.keys[i], data[(i)*16:(i+1)*16]...)
			}
		}
	}
}
