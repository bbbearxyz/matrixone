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

package builtin

import "github.com/matrixorigin/matrixone/pkg/sql/colexec/extend/overload"

const (
	Length = iota + overload.NE + 1
	Space
	Reverse
	Substring
	Ltrim
	Rtrim
	Oct
	StartsWith
	Lpad
	Rpad
	Empty
	LengthUTF8
	Round
	Floor
	Abs
	Log
	Ln
	Ceil
	Exp
	Power
	Pi
	Sin
	Sinh
	Cos
	Acos
	Tan
	Atan
	Cot
	UTCTimestamp
	DayOfYear
	Month
	Year
	Weekday
	EndsWith
	Date
	Bin
	FindInSet
)
