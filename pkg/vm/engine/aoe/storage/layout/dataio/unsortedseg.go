package dataio

import (
	"matrixone/pkg/vm/engine/aoe/storage/common"
	"matrixone/pkg/vm/engine/aoe/storage/layout/base"
	"sync"
	"sync/atomic"
	// log "github.com/sirupsen/logrus"
)

type UnsortedSegmentFile struct {
	sync.RWMutex
	common.RefHelper
	ID     common.ID
	Blocks map[common.ID]base.IBlockFile
	Dir    string
	Info   *fileStat
}

func NewUnsortedSegmentFile(dirname string, id common.ID) base.ISegmentFile {
	usf := &UnsortedSegmentFile{
		ID:     id,
		Dir:    dirname,
		Blocks: make(map[common.ID]base.IBlockFile),
		Info: &fileStat{
			name: id.ToSegmentFilePath(),
		},
	}
	usf.OnZeroCB = usf.close
	return usf
}

func (sf *UnsortedSegmentFile) close() {
	sf.Destory()
}

func (sf *UnsortedSegmentFile) GetFileType() common.FileType {
	return common.DiskFile
}

func (sf *UnsortedSegmentFile) GetDir() string {
	return sf.Dir
}

func (sf *UnsortedSegmentFile) RefBlock(id common.ID) {
	sf.Lock()
	defer sf.Unlock()
	_, ok := sf.Blocks[id]
	if !ok {
		bf := NewBlockFile(sf, id)
		sf.AddBlock(id, bf)
	}
	sf.Ref()
}

func (sf *UnsortedSegmentFile) UnrefBlock(id common.ID) {
	sf.Unref()
}

func (sf *UnsortedSegmentFile) GetIndicesMeta() *base.IndicesMeta {
	return nil
}

func (sf *UnsortedSegmentFile) GetBlockIndicesMeta(id common.ID) *base.IndicesMeta {
	blk := sf.GetBlock(id)
	if blk == nil {
		return nil
	}
	return blk.GetIndicesMeta()
}

func (sf *UnsortedSegmentFile) MakeVirtualIndexFile(meta *base.IndexMeta) common.IVFile {
	return nil
}

func (sf *UnsortedSegmentFile) MakeVirtualBlkIndexFile(id *common.ID, meta *base.IndexMeta) common.IVFile {
	blk := sf.GetBlock(*id)
	if blk == nil {
		return nil
	}
	return blk.MakeVirtualIndexFile(meta)
}

func (sf *UnsortedSegmentFile) MakeVirtualPartFile(id *common.ID) common.IVFile {
	return newPartFile(id, sf, false)
}

func (sf *UnsortedSegmentFile) Stat() common.FileInfo {
	return sf.Info
}

func (sf *UnsortedSegmentFile) Close() error {
	return nil
}

func (sf *UnsortedSegmentFile) Destory() {
	for _, blkFile := range sf.Blocks {
		blkFile.Unref()
	}
	sf.Blocks = nil
}

func (sf *UnsortedSegmentFile) GetBlock(id common.ID) base.IBlockFile {
	sf.RLock()
	defer sf.RUnlock()
	blk := sf.Blocks[id]
	return blk
}

func (sf *UnsortedSegmentFile) AddBlock(id common.ID, bf base.IBlockFile) {
	_, ok := sf.Blocks[id]
	if ok {
		panic("logic error")
	}
	sf.Blocks[id] = bf
	atomic.AddInt64(&sf.Info.size, bf.Stat().Size())
}

func (sf *UnsortedSegmentFile) ReadPoint(ptr *base.Pointer, buf []byte) {
	panic("not supported")
}

func (sf *UnsortedSegmentFile) ReadBlockPoint(id common.ID, ptr *base.Pointer, buf []byte) {
	sf.RLock()
	blk, ok := sf.Blocks[id.AsBlockID()]
	if !ok {
		panic("logic error")
	}
	sf.RUnlock()
	blk.ReadPoint(ptr, buf)
}

func (sf *UnsortedSegmentFile) DataCompressAlgo(id common.ID) int {
	sf.RLock()
	blk, ok := sf.Blocks[id.AsBlockID()]
	if !ok {
		panic("logic error")
	}
	sf.RUnlock()
	return blk.DataCompressAlgo(id)
}

func (sf *UnsortedSegmentFile) PartSize(colIdx uint64, id common.ID, isOrigin bool) int64 {
	sf.RLock()
	blk, ok := sf.Blocks[id.AsBlockID()]
	if !ok {
		panic("logic error")
	}
	sf.RUnlock()
	return blk.PartSize(colIdx, id, isOrigin)
}

func (sf *UnsortedSegmentFile) ReadPart(colIdx uint64, id common.ID, buf []byte) {
	sf.RLock()
	blk, ok := sf.Blocks[id.AsBlockID()]
	if !ok {
		panic("logic error")
	}
	sf.RUnlock()
	blk.ReadPart(colIdx, id, buf)
}
