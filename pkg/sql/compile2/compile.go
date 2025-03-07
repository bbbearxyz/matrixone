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

package compile2

import (
	"github.com/matrixorigin/matrixone/pkg/sql/parsers"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect"
	"github.com/matrixorigin/matrixone/pkg/vm/engine"
	process "github.com/matrixorigin/matrixone/pkg/vm/process2"
)

// InitAddress is used to set address of local node
func InitAddress(addr string) {
	Address = addr
}

// New is used to new an object of compile
func New(db string, sql string, uid string,
	e engine.Engine, proc *process.Process) *compile {
	return &compile{
		e:    e,
		db:   db,
		uid:  uid,
		sql:  sql,
		proc: proc,
	}
}

// Build generates query execution list based on the result of sql parser.
func (c *compile) Build() ([]*Exec, error) {
	stmts, err := parsers.Parse(dialect.MYSQL, c.sql)
	if err != nil {
		return nil, err
	}
	es := make([]*Exec, len(stmts))
	for i := range stmts {
		es[i] = &Exec{
			c:    c,
			stmt: stmts[i],
		}
	}
	return es, nil
}
