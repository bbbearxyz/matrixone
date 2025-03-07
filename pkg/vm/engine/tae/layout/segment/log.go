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

import (
	"bytes"
	"encoding/binary"
)

type Log struct {
	logFile   *BlockFile
	seq       uint64
	offset    uint64
	allocator Allocator
}

func (ex Extent) Replay() {

}

func (l Log) RemoveInode(file *BlockFile) error {
	file.snode.state = REMOVE
	err := l.Append(file)
	if err != nil {
		return err
	}
	l.allocator.Free(file.snode.logExtents.offset, file.snode.logExtents.length)
	return nil
}

func (l Log) Append(file *BlockFile) error {
	var (
		err     error
		ibuffer bytes.Buffer
	)
	segment := l.logFile.segment
	if err = binary.Write(&ibuffer, binary.BigEndian, file.snode.inode); err != nil {
		return err
	}
	if err = binary.Write(&ibuffer, binary.BigEndian, file.snode.algo); err != nil {
		return err
	}
	if err = binary.Write(&ibuffer, binary.BigEndian, file.snode.state); err != nil {
		return err
	}
	if err = binary.Write(&ibuffer, binary.BigEndian, file.snode.size); err != nil {
		return err
	}
	if err = binary.Write(&ibuffer, binary.BigEndian, uint64(len(file.snode.extents))); err != nil {
		return err
	}
	file.snode.mutex.RLock()
	extents := file.snode.extents
	file.snode.mutex.RUnlock()
	for _, ext := range extents {
		if err = binary.Write(&ibuffer, binary.BigEndian, ext.typ); err != nil {
			return err
		}
		if err = binary.Write(&ibuffer, binary.BigEndian, ext.offset); err != nil {
			return err
		}
		if err = binary.Write(&ibuffer, binary.BigEndian, ext.length); err != nil {
			return err
		}
	}
	ibufLen := (segment.super.blockSize - (uint32(ibuffer.Len()) % segment.super.blockSize)) + uint32(ibuffer.Len())
	offset, allocated := l.allocator.Allocate(uint64(ibufLen))
	if _, err = segment.segFile.WriteAt(ibuffer.Bytes(), int64(offset+LOG_START)); err != nil {
		return err
	}
	l.allocator.Free(file.snode.logExtents.offset, file.snode.logExtents.length)
	file.snode.logExtents.offset = uint32(offset)
	file.snode.logExtents.length = uint32(allocated)
	return nil
}
