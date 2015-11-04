// Copyright 2015 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package optimizer_test

import (
	"testing"

	. "github.com/pingcap/check"
	"github.com/pingcap/tidb"
	"github.com/pingcap/tidb/ast"
	"github.com/pingcap/tidb/ast/parser"
	"github.com/pingcap/tidb/context"
	"github.com/pingcap/tidb/model"
	"github.com/pingcap/tidb/optimizer"
	"github.com/pingcap/tidb/sessionctx"
	"github.com/pingcap/tidb/util/testkit"
)

func TestT(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&testInfoBinderSuite{})

type testInfoBinderSuite struct {
}

type binderVerifier struct {
	src string
	c   *C
}

func (bv *binderVerifier) Enter(node ast.Node) (ast.Node, bool) {
	return node, false
}

func (bv *binderVerifier) Leave(in ast.Node) (out ast.Node, ok bool) {
	switch v := in.(type) {
	case *ast.ColumnName:
		bv.c.Assert(v.ColumnInfo, NotNil, Commentf("%s", bv.src))
	case *ast.TableName:
		bv.c.Assert(v.TableInfo, NotNil, Commentf("%s", bv.src))
	}
	return in, true
}

type binderTestCase struct {
	src   string
	valid bool
}

var binderTestCases = []binderTestCase{
	{"select c1 from t1", true},
	{"select c3 from t1", false},
	{"select c1 from t4", false},
	{"select c1 from t1, t2", false},
	{"select * from t1", true},
	{"select t1.* from t1", true},
	{"select t2.* from t1", false},
	{"select c1 as a, c2 as a from t1 group by a", false},
	{"select c1 as a, c1 as a from t1 group by a", true},
	{"select 1 as a, c1 as a, c2 as a from t1 group by a", true},
	{"select c1, c2 as c1 from t1 group by c1", false},
	{"select c1, c2 as c1 from t1 group by c1+1", true},
	{"select c1, c2 as c1 from t1 order by c1", false},
	{"select c1, c2 as c1 from t1 order by c1+1", true},
	{"select * from t1, t2 join t3 on t1.c1 = t2.c1", false},
	{"select * from t1, t2 join t3 on t2.c1 = t3.c1", true},
	{"select c1 from t1 group by c1 having c1 = 3", true},
	{"select c1 from t1 group by c1 having c2 = 3", false},
	{"select c1 from t1 where exists (select c2)", true},
}

func (ts *testInfoBinderSuite) TestInfoBinder(c *C) {
	store, err := tidb.NewStore(tidb.EngineGoLevelDBMemory)
	c.Assert(err, IsNil)
	defer store.Close()
	testKit := testkit.NewTestKit(c, store)
	testKit.MustExec("use test")
	testKit.MustExec("create table t1 (c1 int, c2 int)")
	testKit.MustExec("create table t2 (c1 int, c2 int)")
	testKit.MustExec("create table t3 (c1 int, c2 int)")
	domain := sessionctx.GetDomain(testKit.Se.(context.Context))

	for _, tc := range binderTestCases {
		l := parser.NewLexer(tc.src)
		c.Assert(parser.YYParse(l), Equals, 0)
		stmts := l.Stmts()
		c.Assert(len(stmts), Equals, 1)
		binder := &optimizer.InfoBinder{
			Info:          domain.InfoSchema(),
			DefaultSchema: model.NewCIStr("test"),
		}
		node := stmts[0]
		node.Accept(binder)
		if tc.valid {
			c.Assert(binder.Err, IsNil)
			verifier := &binderVerifier{c: c, src: tc.src}
			node.Accept(verifier)
		} else {
			c.Assert(binder.Err, NotNil, Commentf("%s", tc.src))
		}
	}
}