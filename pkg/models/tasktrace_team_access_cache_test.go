// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamAccessCacheReuseAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	cache := taskTraceTeamAccessCache{now: func() time.Time { return now }}
	scans := 0
	load := func() ([]TaskTraceTeamRepositoryMember, error) {
		scans++
		return []TaskTraceTeamRepositoryMember{{AccountName: "DOMAIN\\alice", Access: TaskTraceTeamAccessWrite}}, nil
	}
	root := t.TempDir()
	members, err := cache.read(root, load)
	require.NoError(t, err)
	members[0].AccountName = "caller mutation"
	for i := 0; i < 20; i++ {
		members, err = cache.read(root, load)
		require.NoError(t, err)
		assert.Equal(t, []TaskTraceTeamRepositoryMember{{AccountName: "DOMAIN\\alice", Access: TaskTraceTeamAccessWrite}}, members)
	}
	assert.Equal(t, 1, scans, "repeated UI reads should launch only one scan")
	now = now.Add(29 * time.Second)
	_, err = cache.read(root, load)
	require.NoError(t, err)
	assert.Equal(t, 1, scans)
	now = now.Add(time.Second)
	_, err = cache.read(root, load)
	require.NoError(t, err)
	assert.Equal(t, 2, scans, "external Explorer permission changes are discovered after 30 seconds")
	cache.invalidate(root)
	_, err = cache.read(root, load)
	require.NoError(t, err)
	assert.Equal(t, 3, scans, "a successful permission mutation must refresh immediately")
}

func TestTaskTraceTeamAccessCacheConcurrentReaders(t *testing.T) {
	var cache taskTraceTeamAccessCache
	root := t.TempDir()
	var scans atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	load := func() ([]TaskTraceTeamRepositoryMember, error) {
		if scans.Add(1) == 1 {
			close(started)
		}
		<-release
		return []TaskTraceTeamRepositoryMember{{AccountName: "DOMAIN\\alice", Access: TaskTraceTeamAccessWrite}}, nil
	}
	var readers sync.WaitGroup
	results := make(chan []TaskTraceTeamRepositoryMember, 24)
	for i := 0; i < 24; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			members, _ := cache.read(root, load)
			results <- members
		}()
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not start")
	}
	// A slow network share must not block reads for a different folder.
	otherRoot := t.TempDir()
	otherDone := make(chan struct{})
	go func() {
		_, _ = cache.read(otherRoot, func() ([]TaskTraceTeamRepositoryMember, error) {
			return []TaskTraceTeamRepositoryMember{{AccountName: "other", Access: TaskTraceTeamAccessRead}}, nil
		})
		close(otherDone)
	}()
	select {
	case <-otherDone:
	case <-time.After(5 * time.Second):
		t.Fatal("unrelated folder was blocked by an in-flight scan")
	}
	close(release)
	readers.Wait()
	close(results)
	for members := range results {
		assert.Equal(t, []TaskTraceTeamRepositoryMember{{AccountName: "DOMAIN\\alice", Access: TaskTraceTeamAccessWrite}}, members)
	}
	assert.EqualValues(t, 1, scans.Load(), "24 simultaneous UI requests should share one Windows scan")
}

func TestTaskTraceTeamAccessCacheFailureRetry(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	cache := taskTraceTeamAccessCache{now: func() time.Time { return now }}
	root := t.TempDir()
	scans := 0
	unavailable := errors.New("temporary network error")
	load := func() ([]TaskTraceTeamRepositoryMember, error) {
		scans++
		if scans == 1 {
			return nil, unavailable
		}
		return []TaskTraceTeamRepositoryMember{{AccountName: "recovered", Access: TaskTraceTeamAccessRead}}, nil
	}
	for i := 0; i < 10; i++ {
		_, err := cache.read(root, load)
		require.ErrorIs(t, err, unavailable)
	}
	assert.Equal(t, 1, scans, "repeated failures should not repeatedly spawn PowerShell")
	now = now.Add(2 * time.Second)
	members, err := cache.read(root, load)
	require.NoError(t, err)
	assert.Equal(t, []TaskTraceTeamRepositoryMember{{AccountName: "recovered", Access: TaskTraceTeamAccessRead}}, members)
	assert.Equal(t, 2, scans)
}

func TestTaskTraceTeamAccessCacheInvalidatesInFlightScan(t *testing.T) {
	var cache taskTraceTeamAccessCache
	root := t.TempDir()
	var scans atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	load := func() ([]TaskTraceTeamRepositoryMember, error) {
		if scans.Add(1) == 1 {
			close(started)
			<-release
			return []TaskTraceTeamRepositoryMember{{AccountName: "removed-user", Access: TaskTraceTeamAccessWrite}}, nil
		}
		return []TaskTraceTeamRepositoryMember{{AccountName: "new-user", Access: TaskTraceTeamAccessRead}}, nil
	}
	result := make(chan []TaskTraceTeamRepositoryMember, 1)
	go func() { members, _ := cache.read(root, load); result <- members }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not start")
	}
	cache.invalidate(root)
	close(release)
	select {
	case members := <-result:
		assert.Equal(t, []TaskTraceTeamRepositoryMember{{AccountName: "new-user", Access: TaskTraceTeamAccessRead}}, members, "stale scan cannot repopulate the list after permissions changed")
	case <-time.After(5 * time.Second):
		t.Fatal("invalidated scan did not retry")
	}
	members, err := cache.read(root, load)
	require.NoError(t, err)
	assert.Equal(t, []TaskTraceTeamRepositoryMember{{AccountName: "new-user", Access: TaskTraceTeamAccessRead}}, members)
	assert.EqualValues(t, 2, scans.Load())
}

func TestTaskTraceTeamAccessCacheEmptyList(t *testing.T) {
	var cache taskTraceTeamAccessCache
	root := t.TempDir()
	for i := 0; i < 2; i++ {
		members, err := cache.read(root, func() ([]TaskTraceTeamRepositoryMember, error) { return []TaskTraceTeamRepositoryMember{}, nil })
		require.NoError(t, err)
		assert.NotNil(t, members, "empty UI candidate lists must remain JSON arrays, not null")
		assert.Empty(t, members)
	}
}

func TestTaskTraceTeamFilterAccessMembersRemovesWindowsSystemGroups(t *testing.T) {
	members := taskTraceTeamFilterAccessMembers([]string{
		`BUILTIN\Administrators`,
		`Administrators`,
		`BUILTIN\管理员`,
		`NT AUTHORITY\SYSTEM`,
		`CREATOR OWNER\CREATOR OWNER`,
		`DOMAIN\alice`,
		`domain\ALICE`,
		`DOMAIN\bob`,
		`DOMAIN\Administrator`,
	})

	assert.Equal(t, []string{`DOMAIN\alice`, `DOMAIN\bob`, `DOMAIN\Administrator`}, members)
}

func TestTaskTraceTeamFilterRepositoryMembersKeepsReadAndWriteLevels(t *testing.T) {
	members := taskTraceTeamFilterRepositoryMembers([]TaskTraceTeamRepositoryMember{
		{AccountName: `BUILTIN\Users`, Access: TaskTraceTeamAccessWrite},
		{AccountName: `DOMAIN\reader`, Access: TaskTraceTeamAccessRead},
		{AccountName: `DOMAIN\writer`, Access: TaskTraceTeamAccessWrite},
	})

	assert.Equal(t, []TaskTraceTeamRepositoryMember{
		{AccountName: `DOMAIN\reader`, Access: TaskTraceTeamAccessRead},
		{AccountName: `DOMAIN\writer`, Access: TaskTraceTeamAccessWrite},
	}, members)
}

func TestTaskTraceTeamAccessCacheCachedNeverStartsAScan(t *testing.T) {
	var cache taskTraceTeamAccessCache
	root := t.TempDir()
	if members, ok := cache.cached(root); ok || members != nil {
		t.Fatal("empty cache unexpectedly returned members")
	}
	_, err := cache.read(root, func() ([]TaskTraceTeamRepositoryMember, error) {
		return []TaskTraceTeamRepositoryMember{{AccountName: "DOMAIN\\alice", Access: TaskTraceTeamAccessWrite}}, nil
	})
	require.NoError(t, err)
	members, ok := cache.cached(root)
	require.True(t, ok)
	assert.Equal(t, []TaskTraceTeamRepositoryMember{{AccountName: "DOMAIN\\alice", Access: TaskTraceTeamAccessWrite}}, members)
}
