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

package ddl

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/pingcap/tidb/context"
	"github.com/pingcap/tidb/kv"
)

var _ context.Context = &reOrgContext{}

type reOrgContext struct {
	store kv.Storage

	m map[fmt.Stringer]interface{}

	txn kv.Transaction
}

func (c *reOrgContext) GetTxn(forceNew bool) (kv.Transaction, error) {
	if forceNew {
		if c.txn != nil {
			if err := c.txn.Commit(); err != nil {
				return nil, errors.Trace(err)
			}
			c.txn = nil
		}
	}

	if c.txn != nil {
		return c.txn, nil
	}

	txn, err := c.store.Begin()
	if err != nil {
		return nil, errors.Trace(err)
	}

	c.txn = txn
	return c.txn, nil
}

func (c *reOrgContext) FinishTxn(rollback bool) error {
	if c.txn == nil {
		return nil
	}

	var err error
	if rollback {
		err = c.txn.Rollback()
	} else {
		err = c.txn.Commit()
	}

	c.txn = nil

	return errors.Trace(err)
}

func (c *reOrgContext) SetValue(key fmt.Stringer, value interface{}) {
	c.m[key] = value
}

func (c *reOrgContext) Value(key fmt.Stringer) interface{} {
	return c.m[key]
}

func (c *reOrgContext) ClearValue(key fmt.Stringer) {
	delete(c.m, key)
}

func (d *ddl) newReOrgContext() context.Context {
	c := &reOrgContext{
		store: d.store,
		m:     make(map[fmt.Stringer]interface{}),
	}

	return c
}

const waitReOrgTimeout = 10 * time.Second

var errWaitReOrgTimeout = errors.New("wait re-orgnization done timeout")

func (d *ddl) runReOrgJob(f func() error) error {
	// wait re-orgnization jobs done
	// TODO use persistent re-orgnization job list.
	if d.reOrgDoneCh == nil {
		// start a re-orgnization job
		d.reOrgDoneCh = make(chan error, 1)

		go func() {
			d.reOrgDoneCh <- f()
		}()
	}

	// wait re-orgnization job done or timeout
	select {
	case err := <-d.reOrgDoneCh:
		d.reOrgDoneCh = nil
		return errors.Trace(err)
	case <-time.After(waitReOrgTimeout):
		// if timeout, we will return, check the owner and retry wait job done again.
		return errWaitReOrgTimeout
	}

}