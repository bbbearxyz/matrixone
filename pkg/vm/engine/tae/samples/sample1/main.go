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

package main

import (
	"os"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/matrixorigin/matrixone/pkg/container/batch"
	"github.com/matrixorigin/matrixone/pkg/logutil"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/catalog"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/container/compute"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/db"
	"github.com/panjf2000/ants/v2"
)

var sampleDir = "/tmp/sample1"
var dbName = "db"
var cpuprofile = "/tmp/sample1/cpuprofile"
var memprofile = "/tmp/sample1/memprofile"

func init() {
	os.RemoveAll(sampleDir)
}

func startProfile() {
	f, _ := os.Create(cpuprofile)
	_ = pprof.StartCPUProfile(f)
}

func stopProfile() {
	pprof.StopCPUProfile()
	memf, _ := os.Create(memprofile)
	defer memf.Close()
	_ = pprof.Lookup("heap").WriteTo(memf, 0)
}

func main() {
	tae, _ := db.Open(sampleDir, nil)
	defer tae.Close()

	schema := catalog.MockSchemaAll(10)
	schema.BlockMaxRows = 80000
	schema.SegmentMaxBlocks = 5
	schema.PrimaryKey = 3
	batchCnt := uint64(200)
	batchRows := uint64(schema.BlockMaxRows) * 1 / 20 * batchCnt
	{
		txn := tae.StartTxn(nil)
		db, _ := txn.CreateDatabase(dbName)
		_, _ = db.CreateRelation(schema)
		if err := txn.Commit(); err != nil {
			panic(err)
		}
	}
	bat := compute.MockBatch(schema.Types(), batchRows, int(schema.PrimaryKey), nil)
	bats := compute.SplitBatch(bat, int(batchCnt))
	var wg sync.WaitGroup
	doAppend := func(b *batch.Batch) func() {
		return func() {
			defer wg.Done()
			txn := tae.StartTxn(nil)
			db, err := txn.GetDatabase(dbName)
			if err != nil {
				panic(err)
			}
			rel, err := db.GetRelationByName(schema.Name)
			if err != nil {
				panic(err)
			}
			if err := rel.Append(b); err != nil {
				panic(err)
			}
			if err := txn.Commit(); err != nil {
				panic(err)
			}
		}
	}
	p, _ := ants.NewPool(200)
	now := time.Now()
	startProfile()
	for _, b := range bats {
		wg.Add(1)
		_ = p.Submit(doAppend(b))
	}
	wg.Wait()
	// {
	// 	txn := mgr.StartTxn(nil)
	// 	db, _ := txn.GetDatabase(dbName)
	// 	rel, _ := db.GetRelationByName(schema.Name)
	// 	var compressed bytes.Buffer
	// 	var decompressed bytes.Buffer
	// 	it := rel.MakeBlockIt()
	// 	for it.Valid() {
	// 		blk := it.GetBlock()
	// 		vec, _ := blk.GetVectorCopy(schema.ColDefs[schema.PrimaryKey].Name, &compressed, &decompressed)
	// 		logutil.Info(vec.String())
	// 		it.Next()
	// 	}
	// }

	stopProfile()
	logutil.Infof("Append takes: %s", time.Since(now))
	// time.Sleep(time.Second * 100)
	logutil.Info(tae.TxnBufMgr.String())
	logutil.Info(tae.MTBufMgr.String())
}
