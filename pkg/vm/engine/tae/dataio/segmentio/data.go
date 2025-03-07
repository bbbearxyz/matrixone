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

package segmentio

import (
	"github.com/matrixorigin/matrixone/pkg/compress"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/layout/segment"
)

type dataFile struct {
	colBlk *columnBlock
	file   []*segment.BlockFile
	buf    []byte
	stat   *fileStat
	cache  []byte
}

type indexFile struct {
	*dataFile
}

type updatesFile struct {
	*dataFile
}

type deletesFile struct {
	block *blockFile
	*dataFile
}

func newData(colBlk *columnBlock) *dataFile {
	df := &dataFile{
		colBlk: colBlk,
		buf:    make([]byte, 0),
	}
	df.stat = &fileStat{}
	return df
}

func newIndex(colBlk *columnBlock) *indexFile {
	index := &indexFile{
		dataFile: newData(colBlk),
	}
	index.dataFile.file = nil
	return index
}

func newUpdates(colBlk *columnBlock) *updatesFile {
	update := &updatesFile{
		dataFile: newData(colBlk),
	}
	update.dataFile.file = nil
	return update
}

func newDeletes(block *blockFile) *deletesFile {
	//col := &columnBlock{block: block, blockType: DELETE}
	del := &deletesFile{
		block:    block,
		dataFile: newData(nil),
	}
	del.dataFile.file = nil
	return del
}

func (df *dataFile) Write(buf []byte) (n int, err error) {
	if df.file == nil {
		n = len(buf)
		df.buf = make([]byte, len(buf))
		copy(df.buf, buf)
		df.stat.size = int64(len(df.buf))
		df.stat.algo = 0
		df.stat.originSize = int64(len(df.buf))
		return
	}
	df.colBlk.mutex.RLock()
	file := df.file[len(df.file)-1]
	df.colBlk.mutex.RUnlock()
	err = file.GetSegement().Append(file, buf)
	df.stat.algo = compress.Lz4
	df.stat.originSize = file.GetOriginSize()
	df.stat.size = file.GetFileSize()
	return
}

func (df *dataFile) Read(buf []byte) (n int, err error) {
	if df.file == nil {
		n = len(buf)
		copy(buf, df.buf)
		return
	}
	bufLen := len(buf)
	if bufLen == 0 {
		return 0, nil
	}
	df.colBlk.mutex.RLock()
	file := df.file[len(df.file)-1]
	df.colBlk.mutex.RUnlock()
	n, err = file.Read(buf)
	return n, nil
}

func (df *dataFile) GetFileType() common.FileType {
	return common.DiskFile
}

func (df *dataFile) Ref()            { df.colBlk.Ref() }
func (df *dataFile) Unref()          { df.colBlk.Unref() }
func (df *dataFile) RefCount() int64 { return df.colBlk.RefCount() }

func (df *dataFile) Stat() common.FileInfo { return df.stat }

func (df *deletesFile) Ref()   { df.block.Ref() }
func (df *deletesFile) Unref() { df.block.Unref() }
