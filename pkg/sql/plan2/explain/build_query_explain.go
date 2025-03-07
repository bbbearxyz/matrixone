// Copyright 2021 - 2022 Matrix Origin
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

package explain

import (
	"github.com/matrixorigin/matrixone/pkg/errno"
	"github.com/matrixorigin/matrixone/pkg/logutil"
	"github.com/matrixorigin/matrixone/pkg/pb/plan"
	"github.com/matrixorigin/matrixone/pkg/sql/errors"
)

var _ ExplainQuery = &ExplainQueryImpl{}

type ExplainQueryImpl struct {
	QueryPlan *plan.Query
}

func NewExplainQueryImpl(query *plan.Query) *ExplainQueryImpl {
	return &ExplainQueryImpl{
		QueryPlan: query,
	}
}

func (e *ExplainQueryImpl) ExplainPlan(buffer *ExplainDataBuffer, options *ExplainOptions) error {
	var Nodes []*plan.Node = e.QueryPlan.Nodes
	for index, rootNodeId := range e.QueryPlan.Steps {
		logutil.Infof("------------------------------------Query Plan-%v ---------------------------------------------", index)
		settings := FormatSettings{
			buffer: buffer,
			offset: 0,
			indent: 2,
			level:  0,
		}
		err := traversalPlan(Nodes[rootNodeId], Nodes, &settings, options)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *ExplainQueryImpl) ExplainAnalyze(buffer *ExplainDataBuffer, options *ExplainOptions) error {
	//TODO implement me
	panic("implement me")
}

func explainStep(step *plan.Node, settings *FormatSettings, options *ExplainOptions) error {
	nodedescImpl := NewNodeDescriptionImpl(step)

	if options.Format == EXPLAIN_FORMAT_TEXT {
		basicNodeInfo, err := nodedescImpl.GetNodeBasicInfo(options)
		if err != nil {
			return nil
		}
		settings.buffer.PushNewLine(basicNodeInfo, true, settings.level)

		// Process verbose optioan information , "Output:"
		if options.Verbose {
			if nodedescImpl.Node.GetProjectList() != nil {
				projecrtInfo, err := nodedescImpl.GetProjectListInfo(options)
				if err != nil {
					return err
				}
				settings.buffer.PushNewLine(projecrtInfo, false, settings.level)
			}

			if nodedescImpl.Node.NodeType == plan.Node_VALUE_SCAN {
				rowsetDataDescImpl := &RowsetDataDescribeImpl{
					RowsetData: nodedescImpl.Node.RowsetData,
				}
				rowsetInfo, err := rowsetDataDescImpl.GetDescription(options)
				if err != nil {
					return err
				}
				rowdatadesc := "Output: " + rowsetInfo
				settings.buffer.PushNewLine(rowdatadesc, false, settings.level)
			}
		}

		// Get other node descriptions, such as "Filter:", "Group Key:", "Sort Key:"
		extraInfo, err := nodedescImpl.GetExtraInfo(options)
		if err != nil {
			return err
		}
		for _, line := range extraInfo {
			settings.buffer.PushNewLine(line, false, settings.level)
		}
	} else if options.Format == EXPLAIN_FORMAT_JSON {
		return errors.New(errno.FeatureNotSupported, "unimplement explain format json")
	} else if options.Format == EXPLAIN_FORMAT_DOT {
		return errors.New(errno.FeatureNotSupported, "unimplement explain format dot")
	}
	return nil
}

func traversalPlan(node *plan.Node, Nodes []*plan.Node, settings *FormatSettings, options *ExplainOptions) error {
	if node == nil {
		return nil
	}
	err := explainStep(node, settings, options)
	if err != nil {
		return err
	}
	settings.level++
	// Recursive traversal Query Plan
	if len(node.Children) > 0 {
		for _, childIndex := range node.Children {
			err = traversalPlan(Nodes[childIndex], Nodes, settings, options)
			if err != nil {
				return err
			}
		}
	}
	settings.level--
	return nil
}
