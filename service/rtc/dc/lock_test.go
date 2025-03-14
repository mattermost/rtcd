// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package dc

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewLock(t *testing.T) {
	lock := NewLock()
	require.NotNil(t, lock)
	require.NotNil(t, lock.syncCh)
}

func TestLockLock(t *testing.T) {
	t.Run("successful lock", func(t *testing.T) {
		lock := NewLock()
		err := lock.Lock(100 * time.Millisecond)
		require.NoError(t, err)
	})

	t.Run("timeout", func(t *testing.T) {
		lock := NewLock()
		// First lock should succeed
		err := lock.Lock(100 * time.Millisecond)
		require.NoError(t, err)

		// Second lock should timeout
		err = lock.Lock(100 * time.Millisecond)
		require.Error(t, err)
		require.Equal(t, ErrLockTimeout, err)

		err = lock.Unlock()
		require.NoError(t, err)

		// Third lock should succeed
		err = lock.Lock(100 * time.Millisecond)
		require.NoError(t, err)
	})
}

func TestLockUnlock(t *testing.T) {
	t.Run("successful unlock", func(t *testing.T) {
		lock := NewLock()
		// First acquire the lock
		err := lock.Lock(100 * time.Millisecond)
		require.NoError(t, err)

		// Then unlock it
		err = lock.Unlock()
		require.NoError(t, err)

		// Should be able to lock again
		err = lock.Lock(100 * time.Millisecond)
		require.NoError(t, err)
	})

	t.Run("already unlocked", func(t *testing.T) {
		lock := NewLock()
		// Lock is initially available (unlocked)
		// Trying to unlock should return error
		err := lock.Unlock()
		require.Error(t, err)
		require.Equal(t, ErrAlreadyUnlocked, err)
	})
}

func TestLockTryLock(t *testing.T) {
	t.Run("successful trylock", func(t *testing.T) {
		lock := NewLock()
		// Lock should be initially available
		success := lock.TryLock()
		require.True(t, success, "TryLock should succeed on a new lock")

		// Trying again should fail since we already have the lock
		success = lock.TryLock()
		require.False(t, success, "TryLock should fail when lock is already acquired")

		// After unlocking, TryLock should succeed again
		err := lock.Unlock()
		require.NoError(t, err)

		success = lock.TryLock()
		require.True(t, success, "TryLock should succeed after unlocking")
	})

	t.Run("trylock with concurrent access", func(t *testing.T) {
		lock := NewLock()

		// First goroutine acquires the lock
		success := lock.TryLock()
		require.True(t, success)

		// Use a channel to coordinate between goroutines
		done := make(chan bool)

		go func() {
			// This should fail since the lock is held by the main goroutine
			success := lock.TryLock()
			require.False(t, success, "TryLock should fail when lock is held by another goroutine")

			// Wait for notification that lock has been released
			<-done

			// Now it should succeed
			success = lock.TryLock()
			require.True(t, success, "TryLock should succeed after lock is released")

			// Release the lock
			err := lock.Unlock()
			require.NoError(t, err)

			done <- true
		}()

		// Give the goroutine time to try and fail to acquire the lock
		time.Sleep(50 * time.Millisecond)

		// Release the lock
		err := lock.Unlock()
		require.NoError(t, err)

		// Notify goroutine that lock has been released
		done <- true

		// Wait for goroutine to finish
		<-done
	})
}

func TestLockConcurrency(t *testing.T) {
	t.Run("sequential acquisition", func(t *testing.T) {
		lock := NewLock()

		// Test that multiple goroutines can acquire the lock in sequence
		done := make(chan bool)

		go func() {
			err := lock.Lock(100 * time.Millisecond)
			require.NoError(t, err)

			// Hold the lock for a short time
			time.Sleep(50 * time.Millisecond)

			err = lock.Unlock()
			require.NoError(t, err)

			done <- true
		}()

		// Wait for first goroutine to finish
		<-done

		// Second goroutine should now be able to acquire the lock
		err := lock.Lock(100 * time.Millisecond)
		require.NoError(t, err)

		err = lock.Unlock()
		require.NoError(t, err)
	})

	t.Run("multiple goroutines competing", func(t *testing.T) {
		lock := NewLock()
		numGoroutines := 5
		acquiredCount := int32(0)
		timeoutCount := int32(0)

		// Use a WaitGroup to wait for all goroutines to complete
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		// Start multiple goroutines that all try to acquire the lock
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()

				// Try to acquire the lock with a timeout
				err := lock.Lock(200 * time.Millisecond)
				if err == nil {
					// Successfully acquired the lock
					atomic.AddInt32(&acquiredCount, 1)

					// Hold the lock briefly
					time.Sleep(10 * time.Millisecond)

					// Release the lock
					err = lock.Unlock()
					require.NoError(t, err)
				} else {
					// Timed out waiting for the lock
					require.Equal(t, ErrLockTimeout, err)
					atomic.AddInt32(&timeoutCount, 1)
				}
			}()
		}

		// Wait for all goroutines to finish
		wg.Wait()

		// Verify that at least one goroutine acquired the lock
		// and the sum of acquired and timeout counts equals the total number of goroutines
		require.Greater(t, acquiredCount, int32(0))
		require.Equal(t, int32(numGoroutines), acquiredCount+timeoutCount)
	})
}
