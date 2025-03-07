// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package dc

import (
	"errors"
	"time"
)

var (
	ErrLockTimeout     = errors.New("lock timeout")
	ErrAlreadyUnlocked = errors.New("already unlocked")
)

type Lock struct {
	syncCh chan struct{}
}

func NewLock() *Lock {
	syncCh := make(chan struct{}, 1)
	syncCh <- struct{}{}
	return &Lock{
		syncCh: syncCh,
	}
}

func (l *Lock) Lock(timeout time.Duration) error {
	select {
	case <-l.syncCh:
		return nil
	case <-time.After(timeout):
		return ErrLockTimeout
	}
}

func (l *Lock) TryLock() bool {
	select {
	case <-l.syncCh:
		return true
	default:
		return false
	}
}

func (l *Lock) Unlock() error {
	select {
	case l.syncCh <- struct{}{}:
		return nil
	default:
		return ErrAlreadyUnlocked
	}
}
