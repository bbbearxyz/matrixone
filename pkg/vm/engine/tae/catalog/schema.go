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

package catalog

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"time"

	"github.com/matrixorigin/matrixone/pkg/common/moerr"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/vm/engine"

	pkgcatalog "github.com/matrixorigin/matrixone/pkg/catalog"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/containers"
)

func i82bool(v int8) bool {
	return v == 1
}

type ColDef struct {
	Name          string
	Idx           int // indicates its position in all coldefs
	Type          types.Type
	Hidden        bool // Hidden Column is generated by compute layer, keep hidden from user
	PhyAddr       bool // PhyAddr Column is generated by tae as rowid
	NullAbility   bool
	AutoIncrement bool
	Primary       bool
	SortIdx       int8 // indicates its position in all sort keys
	SortKey       bool
	Comment       string
	ClusterBy     bool
	Default       []byte
	OnUpdate      []byte
}

func (def *ColDef) GetName() string     { return def.Name }
func (def *ColDef) GetType() types.Type { return def.Type }

func (def *ColDef) Nullable() bool        { return def.NullAbility }
func (def *ColDef) IsHidden() bool        { return def.Hidden }
func (def *ColDef) IsPhyAddr() bool       { return def.PhyAddr }
func (def *ColDef) IsPrimary() bool       { return def.Primary }
func (def *ColDef) IsAutoIncrement() bool { return def.AutoIncrement }
func (def *ColDef) IsSortKey() bool       { return def.SortKey }
func (def *ColDef) IsClusterBy() bool     { return def.ClusterBy }

type SortKey struct {
	Defs      []*ColDef
	search    map[int]int
	isPrimary bool
}

func NewSortKey() *SortKey {
	return &SortKey{
		Defs:   make([]*ColDef, 0),
		search: make(map[int]int),
	}
}

func (cpk *SortKey) AddDef(def *ColDef) (ok bool) {
	_, found := cpk.search[def.Idx]
	if found {
		return false
	}
	if def.IsPrimary() {
		cpk.isPrimary = true
	}
	cpk.Defs = append(cpk.Defs, def)
	sort.Slice(cpk.Defs, func(i, j int) bool { return cpk.Defs[i].SortIdx < cpk.Defs[j].SortIdx })
	cpk.search[def.Idx] = int(def.SortIdx)
	return true
}

func (cpk *SortKey) IsPrimary() bool                { return cpk.isPrimary }
func (cpk *SortKey) Size() int                      { return len(cpk.Defs) }
func (cpk *SortKey) GetDef(pos int) *ColDef         { return cpk.Defs[pos] }
func (cpk *SortKey) HasColumn(idx int) (found bool) { _, found = cpk.search[idx]; return }
func (cpk *SortKey) GetSingleIdx() int              { return cpk.Defs[0].Idx }

type Schema struct {
	AcInfo           accessInfo
	Name             string
	ColDefs          []*ColDef
	NameIndex        map[string]int
	BlockMaxRows     uint32
	SegmentMaxBlocks uint16
	Comment          string
	Partition        string
	Relkind          string
	Createsql        string
	View             string
	UniqueIndex      string
	SecondaryIndex   string
	Constraint       []byte

	SortKey    *SortKey
	PhyAddrKey *ColDef
}

func NewEmptySchema(name string) *Schema {
	return &Schema{
		Name:      name,
		ColDefs:   make([]*ColDef, 0),
		NameIndex: make(map[string]int),
	}
}

func (s *Schema) Clone() *Schema {
	buf, err := s.Marshal()
	if err != nil {
		panic(err)
	}
	ns := NewEmptySchema(s.Name)
	r := bytes.NewBuffer(buf)
	if _, err = ns.ReadFrom(r); err != nil {
		panic(err)
	}
	return ns
}

func (s *Schema) HasPK() bool      { return s.SortKey != nil && s.SortKey.IsPrimary() }
func (s *Schema) HasSortKey() bool { return s.SortKey != nil }

// GetSingleSortKey should be call only if IsSinglePK is checked
func (s *Schema) GetSingleSortKey() *ColDef        { return s.SortKey.Defs[0] }
func (s *Schema) GetSingleSortKeyIdx() int         { return s.SortKey.Defs[0].Idx }
func (s *Schema) GetSingleSortKeyType() types.Type { return s.GetSingleSortKey().Type }

func (s *Schema) ReadFrom(r io.Reader) (n int64, err error) {
	if err = binary.Read(r, binary.BigEndian, &s.BlockMaxRows); err != nil {
		return
	}
	if err = binary.Read(r, binary.BigEndian, &s.SegmentMaxBlocks); err != nil {
		return
	}
	n = 4 + 4
	var sn int64
	if sn, err = s.AcInfo.ReadFrom(r); err != nil {
		return
	}
	n += sn
	if s.Name, sn, err = common.ReadString(r); err != nil {
		return
	}
	n += sn
	if s.Comment, sn, err = common.ReadString(r); err != nil {
		return
	}
	n += sn
	if s.Partition, sn, err = common.ReadString(r); err != nil {
		return
	}
	n += sn
	if s.Relkind, sn, err = common.ReadString(r); err != nil {
		return
	}
	n += sn
	if s.Createsql, sn, err = common.ReadString(r); err != nil {
		return
	}
	n += sn

	if s.View, sn, err = common.ReadString(r); err != nil {
		return
	}
	if s.UniqueIndex, sn, err = common.ReadString(r); err != nil {
		return
	}
	n += sn
	if s.SecondaryIndex, sn, err = common.ReadString(r); err != nil {
		return
	}
	n += sn
	if s.Constraint, sn, err = common.ReadBytes(r); err != nil {
		return
	}
	n += sn
	colCnt := uint16(0)
	if err = binary.Read(r, binary.BigEndian, &colCnt); err != nil {
		return
	}
	n += 2
	colBuf := make([]byte, types.TSize)
	for i := uint16(0); i < colCnt; i++ {
		if _, err = r.Read(colBuf); err != nil {
			return
		}
		n += int64(types.TSize)
		def := new(ColDef)
		def.Type = types.DecodeType(colBuf)
		if def.Name, sn, err = common.ReadString(r); err != nil {
			return
		}
		n += sn
		if def.Comment, sn, err = common.ReadString(r); err != nil {
			return
		}
		n += sn
		if err = binary.Read(r, binary.BigEndian, &def.NullAbility); err != nil {
			return
		}
		n += 1
		if err = binary.Read(r, binary.BigEndian, &def.Hidden); err != nil {
			return
		}
		n += 1
		if err = binary.Read(r, binary.BigEndian, &def.PhyAddr); err != nil {
			return
		}
		n += 1
		if err = binary.Read(r, binary.BigEndian, &def.AutoIncrement); err != nil {
			return
		}
		n += 1
		if err = binary.Read(r, binary.BigEndian, &def.SortIdx); err != nil {
			return
		}
		n += 1
		if err = binary.Read(r, binary.BigEndian, &def.Primary); err != nil {
			return
		}
		n += 1
		if err = binary.Read(r, binary.BigEndian, &def.SortKey); err != nil {
			return
		}
		n += 1
		if err = binary.Read(r, binary.BigEndian, &def.ClusterBy); err != nil {
			return
		}
		n += 1
		length := uint64(0)
		if err = binary.Read(r, binary.BigEndian, &length); err != nil {
			return
		}
		n += 8
		def.Default = make([]byte, length)
		var sn2 int
		if sn2, err = r.Read(def.Default); err != nil {
			return
		}
		n += int64(sn2)

		length = uint64(0)
		if err = binary.Read(r, binary.BigEndian, &length); err != nil {
			return
		}
		n += 8
		def.OnUpdate = make([]byte, length)
		if sn2, err = r.Read(def.OnUpdate); err != nil {
			return
		}
		n += int64(sn2)
		if err = s.AppendColDef(def); err != nil {
			return
		}
	}
	err = s.Finalize(true)
	return
}

func (s *Schema) Marshal() (buf []byte, err error) {
	var w bytes.Buffer
	if err = binary.Write(&w, binary.BigEndian, s.BlockMaxRows); err != nil {
		return
	}
	if err = binary.Write(&w, binary.BigEndian, s.SegmentMaxBlocks); err != nil {
		return
	}
	if _, err = s.AcInfo.WriteTo(&w); err != nil {
		return
	}
	if _, err = common.WriteString(s.Name, &w); err != nil {
		return
	}
	if _, err = common.WriteString(s.Comment, &w); err != nil {
		return
	}
	if _, err = common.WriteString(s.Partition, &w); err != nil {
		return
	}
	if _, err = common.WriteString(s.Relkind, &w); err != nil {
		return
	}
	if _, err = common.WriteString(s.Createsql, &w); err != nil {
		return
	}
	if _, err = common.WriteString(s.View, &w); err != nil {
		return
	}
	if _, err = common.WriteBytes(s.Constraint, &w); err != nil {
		return
	}
	if _, err = common.WriteString(s.UniqueIndex, &w); err != nil {
		return
	}
	if _, err = common.WriteString(s.SecondaryIndex, &w); err != nil {
		return
	}
	if err = binary.Write(&w, binary.BigEndian, uint16(len(s.ColDefs))); err != nil {
		return
	}
	for _, def := range s.ColDefs {
		if _, err = w.Write(types.EncodeType(&def.Type)); err != nil {
			return
		}
		if _, err = common.WriteString(def.Name, &w); err != nil {
			return
		}
		if _, err = common.WriteString(def.Comment, &w); err != nil {
			return
		}
		if err = binary.Write(&w, binary.BigEndian, def.NullAbility); err != nil {
			return
		}
		if err = binary.Write(&w, binary.BigEndian, def.Hidden); err != nil {
			return
		}
		if err = binary.Write(&w, binary.BigEndian, def.PhyAddr); err != nil {
			return
		}
		if err = binary.Write(&w, binary.BigEndian, def.AutoIncrement); err != nil {
			return
		}
		if err = binary.Write(&w, binary.BigEndian, def.SortIdx); err != nil {
			return
		}
		if err = binary.Write(&w, binary.BigEndian, def.Primary); err != nil {
			return
		}
		if err = binary.Write(&w, binary.BigEndian, def.SortKey); err != nil {
			return
		}
		if err = binary.Write(&w, binary.BigEndian, def.ClusterBy); err != nil {
			return
		}
		length := uint64(len(def.Default))
		if err = binary.Write(&w, binary.BigEndian, length); err != nil {
			return
		}
		if _, err = w.Write(def.Default); err != nil {
			return
		}
		length = uint64(len(def.OnUpdate))
		if err = binary.Write(&w, binary.BigEndian, length); err != nil {
			return
		}
		if _, err = w.Write(def.OnUpdate); err != nil {
			return
		}
	}
	buf = w.Bytes()
	return
}

func (s *Schema) ReadFromBatch(bat *containers.Batch, offset int) (next int) {
	nameVec := bat.GetVectorByName(pkgcatalog.SystemColAttr_RelName)
	tidVec := bat.GetVectorByName(pkgcatalog.SystemColAttr_RelID)
	tid := tidVec.Get(offset).(uint64)
	for {
		if offset >= nameVec.Length() {
			break
		}
		name := string(nameVec.Get(offset).([]byte))
		id := tidVec.Get(offset).(uint64)
		if name != s.Name || id != tid {
			break
		}
		def := new(ColDef)
		def.Name = string(bat.GetVectorByName((pkgcatalog.SystemColAttr_Name)).Get(offset).([]byte))
		data := bat.GetVectorByName((pkgcatalog.SystemColAttr_Type)).Get(offset).([]byte)
		types.Decode(data, &def.Type)
		nullable := bat.GetVectorByName((pkgcatalog.SystemColAttr_NullAbility)).Get(offset).(int8)
		def.NullAbility = i82bool(nullable)
		isHidden := bat.GetVectorByName((pkgcatalog.SystemColAttr_IsHidden)).Get(offset).(int8)
		def.Hidden = i82bool(isHidden)
		isClusterBy := bat.GetVectorByName((pkgcatalog.SystemColAttr_IsClusterBy)).Get(offset).(int8)
		def.ClusterBy = i82bool(isClusterBy)
		isAutoIncrement := bat.GetVectorByName((pkgcatalog.SystemColAttr_IsAutoIncrement)).Get(offset).(int8)
		def.AutoIncrement = i82bool(isAutoIncrement)
		def.Comment = string(bat.GetVectorByName((pkgcatalog.SystemColAttr_Comment)).Get(offset).([]byte))
		def.OnUpdate = bat.GetVectorByName((pkgcatalog.SystemColAttr_Update)).Get(offset).([]byte)
		def.Default = bat.GetVectorByName((pkgcatalog.SystemColAttr_DefaultExpr)).Get(offset).([]byte)
		def.Idx = int(bat.GetVectorByName((pkgcatalog.SystemColAttr_Num)).Get(offset).(int32)) - 1
		s.NameIndex[def.Name] = def.Idx
		s.ColDefs = append(s.ColDefs, def)
		if def.Name == PhyAddrColumnName {
			def.PhyAddr = true
		}
		constraint := string(bat.GetVectorByName(pkgcatalog.SystemColAttr_ConstraintType).Get(offset).([]byte))
		if constraint == "p" {
			def.SortKey = true
			def.Primary = true
		}
		offset++
	}
	s.Finalize(true)
	return offset
}

func (s *Schema) AppendColDef(def *ColDef) (err error) {
	def.Idx = len(s.ColDefs)
	s.ColDefs = append(s.ColDefs, def)
	_, existed := s.NameIndex[def.Name]
	if existed {
		err = moerr.NewConstraintViolationNoCtx("duplicate column \"%s\"", def.Name)
		return
	}
	s.NameIndex[def.Name] = def.Idx
	return
}

func (s *Schema) AppendCol(name string, typ types.Type) error {
	def := &ColDef{
		Name:        name,
		Type:        typ,
		SortIdx:     -1,
		NullAbility: true,
	}
	return s.AppendColDef(def)
}

func (s *Schema) AppendSortKey(name string, typ types.Type, idx int, isPrimary bool) error {
	def := &ColDef{
		Name:    name,
		Type:    typ,
		SortIdx: int8(idx),
		SortKey: true,
	}
	def.Primary = isPrimary
	return s.AppendColDef(def)
}

func (s *Schema) AppendPKCol(name string, typ types.Type, idx int) error {
	def := &ColDef{
		Name:        name,
		Type:        typ,
		SortIdx:     int8(idx),
		SortKey:     true,
		Primary:     true,
		NullAbility: false,
	}
	return s.AppendColDef(def)
}

// non-cn doesn't set IsPrimary in attr, so isPrimary is used explicitly here
func (s *Schema) AppendSortColWithAttribute(attr engine.Attribute, sorIdx int, isPrimary bool) error {
	def, err := ColDefFromAttribute(attr)
	if err != nil {
		return err
	}

	def.SortKey = true
	def.SortIdx = int8(sorIdx)
	def.Primary = isPrimary

	return s.AppendColDef(def)
}

// make a basic coldef without sortKey info
func ColDefFromAttribute(attr engine.Attribute) (*ColDef, error) {
	var err error
	def := &ColDef{
		Name:          attr.Name,
		Type:          attr.Type,
		Hidden:        attr.IsHidden,
		SortIdx:       -1,
		Comment:       attr.Comment,
		AutoIncrement: attr.AutoIncrement,
		ClusterBy:     attr.ClusterBy,
		Default:       []byte(""),
		OnUpdate:      []byte(""),
	}
	if attr.Default != nil {
		def.NullAbility = attr.Default.NullAbility
		if def.Default, err = types.Encode(attr.Default); err != nil {
			return nil, err
		}
	}
	if attr.OnUpdate != nil {
		if def.OnUpdate, err = types.Encode(attr.OnUpdate); err != nil {
			return nil, err
		}
	}
	return def, nil
}

func (s *Schema) AppendColWithAttribute(attr engine.Attribute) error {
	def, err := ColDefFromAttribute(attr)
	if err != nil {
		return err
	}
	return s.AppendColDef(def)
}

func (s *Schema) String() string {
	buf, _ := json.Marshal(s)
	return string(buf)
}

func (s *Schema) Attrs() []string {
	if len(s.ColDefs) == 0 {
		return make([]string, 0)
	}
	attrs := make([]string, 0, len(s.ColDefs)-1)
	for _, def := range s.ColDefs {
		if def.IsPhyAddr() {
			continue
		}
		attrs = append(attrs, def.Name)
	}
	return attrs
}

func (s *Schema) Types() []types.Type {
	if len(s.ColDefs) == 0 {
		return make([]types.Type, 0)
	}
	ts := make([]types.Type, 0, len(s.ColDefs)-1)
	for _, def := range s.ColDefs {
		if def.IsPhyAddr() {
			continue
		}
		ts = append(ts, def.Type)
	}
	return ts
}

func (s *Schema) Nullables() []bool {
	if len(s.ColDefs) == 0 {
		return make([]bool, 0)
	}
	nulls := make([]bool, 0, len(s.ColDefs)-1)
	for _, def := range s.ColDefs {
		if def.IsPhyAddr() {
			continue
		}
		nulls = append(nulls, def.Nullable())
	}
	return nulls
}

func (s *Schema) AllNullables() []bool {
	if len(s.ColDefs) == 0 {
		return make([]bool, 0)
	}
	nulls := make([]bool, 0, len(s.ColDefs))
	for _, def := range s.ColDefs {
		nulls = append(nulls, def.Nullable())
	}
	return nulls
}

func (s *Schema) AllTypes() []types.Type {
	if len(s.ColDefs) == 0 {
		return make([]types.Type, 0)
	}
	ts := make([]types.Type, 0, len(s.ColDefs))
	for _, def := range s.ColDefs {
		ts = append(ts, def.Type)
	}
	return ts
}

func (s *Schema) AllNames() []string {
	if len(s.ColDefs) == 0 {
		return make([]string, 0)
	}
	names := make([]string, 0, len(s.ColDefs))
	for _, def := range s.ColDefs {
		names = append(names, def.Name)
	}
	return names
}

// Finalize runs various checks and create shortcuts to phyaddr and sortkey
func (s *Schema) Finalize(withoutPhyAddr bool) (err error) {
	if s == nil {
		err = moerr.NewConstraintViolationNoCtx("no schema")
		return
	}
	if !withoutPhyAddr {
		phyAddrDef := &ColDef{
			Name:        PhyAddrColumnName,
			Comment:     PhyAddrColumnComment,
			Type:        PhyAddrColumnType,
			Hidden:      true,
			NullAbility: false,
			PhyAddr:     true,
		}
		if err = s.AppendColDef(phyAddrDef); err != nil {
			return
		}
	}
	if len(s.ColDefs) == 0 {
		err = moerr.NewConstraintViolationNoCtx("empty column defs")
		return
	}

	// sortColIdx is sort key index list. as of now, sort key is pk
	sortColIdx := make([]int, 0)
	// check duplicate column names
	names := make(map[string]bool)
	for idx, def := range s.ColDefs {
		// Check column sequence idx validility
		if idx != def.Idx {
			return moerr.NewInvalidInputNoCtx(fmt.Sprintf("schema: wrong column index %d specified for \"%s\"", def.Idx, def.Name))
		}
		// Check unique name
		if _, ok := names[def.Name]; ok {
			return moerr.NewInvalidInputNoCtx("schema: duplicate column \"%s\"", def.Name)
		}
		names[def.Name] = true
		if def.IsSortKey() {
			sortColIdx = append(sortColIdx, idx)
		}
		if def.IsPhyAddr() {
			if s.PhyAddrKey != nil {
				return moerr.NewInvalidInputNoCtx("schema: duplicated physical address column \"%s\"", def.Name)
			}
			s.PhyAddrKey = def
		}
	}

	if len(sortColIdx) == 1 {
		def := s.ColDefs[sortColIdx[0]]
		if def.SortIdx != 0 {
			err = moerr.NewConstraintViolationNoCtx("bad sort idx %d, should be 0", def.SortIdx)
			return
		}
		s.SortKey = NewSortKey()
		s.SortKey.AddDef(def)
	} else if len(sortColIdx) > 1 {
		// schema has a primary key or a cluster by key, or nothing for now
		panic("schema: multiple sort keys")
	}
	return
}

// GetColIdx returns column index for the given column name
// if found, otherwise returns -1.
func (s *Schema) GetColIdx(attr string) int {
	idx, ok := s.NameIndex[attr]
	if !ok {
		return -1
	}
	return idx
}

func GetAttrIdx(attrs []string, name string) int {
	for i, attr := range attrs {
		if attr == name {
			return i
		}
	}
	panic("logic error")
}

func MockSchema(colCnt int, pkIdx int) *Schema {
	rand.Seed(time.Now().UnixNano())
	schema := NewEmptySchema(time.Now().String())
	prefix := "mock_"
	for i := 0; i < colCnt; i++ {
		if pkIdx == i {
			_ = schema.AppendPKCol(fmt.Sprintf("%s%d", prefix, i), types.Type{Oid: types.T_int32, Size: 4, Width: 4}, 0)
		} else {
			_ = schema.AppendCol(fmt.Sprintf("%s%d", prefix, i), types.Type{Oid: types.T_int32, Size: 4, Width: 4})
		}
	}
	_ = schema.Finalize(false)
	return schema
}

// MockSchemaAll if char/varchar is needed, colCnt = 14, otherwise colCnt = 12
// pkIdx == -1 means no pk defined
func MockSchemaAll(colCnt int, pkIdx int, from ...int) *Schema {
	schema := NewEmptySchema(time.Now().String())
	prefix := "mock_"
	start := 0
	if len(from) > 0 {
		start = from[0]
	}
	for i := 0; i < colCnt; i++ {
		if i < start {
			continue
		}
		name := fmt.Sprintf("%s%d", prefix, i)
		var typ types.Type
		switch i % 18 {
		case 0:
			typ = types.T_int8.ToType()
			typ.Width = 8
		case 1:
			typ = types.T_int16.ToType()
			typ.Width = 16
		case 2:
			typ = types.T_int32.ToType()
			typ.Width = 32
		case 3:
			typ = types.T_int64.ToType()
			typ.Width = 64
		case 4:
			typ = types.T_uint8.ToType()
			typ.Width = 8
		case 5:
			typ = types.T_uint16.ToType()
			typ.Width = 16
		case 6:
			typ = types.T_uint32.ToType()
			typ.Width = 32
		case 7:
			typ = types.T_uint64.ToType()
			typ.Width = 64
		case 8:
			typ = types.T_float32.ToType()
			typ.Width = 32
		case 9:
			typ = types.T_float64.ToType()
			typ.Width = 64
		case 10:
			typ = types.T_date.ToType()
			typ.Width = 32
		case 11:
			typ = types.T_datetime.ToType()
			typ.Width = 64
		case 12:
			typ = types.T_varchar.ToType()
			typ.Width = 100
		case 13:
			typ = types.T_char.ToType()
			typ.Width = 100
		case 14:
			typ = types.T_timestamp.ToType()
			typ.Width = 64
		case 15:
			typ = types.T_decimal64.ToType()
			typ.Width = 64
		case 16:
			typ = types.T_decimal128.ToType()
			typ.Width = 128
		case 17:
			typ = types.T_bool.ToType()
			typ.Width = 8
		}

		if pkIdx == i {
			_ = schema.AppendPKCol(name, typ, 0)
		} else {
			_ = schema.AppendCol(name, typ)
			schema.ColDefs[len(schema.ColDefs)-1].NullAbility = true
		}
	}
	schema.BlockMaxRows = 1000
	schema.SegmentMaxBlocks = 10
	_ = schema.Finalize(false)
	return schema
}
