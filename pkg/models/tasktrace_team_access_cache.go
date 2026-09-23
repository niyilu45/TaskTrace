// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// This cache is only for the candidate list shown in the UI. Granting or
// revoking access still runs the real Windows permission operation every time.
type taskTraceTeamAccessCache struct {
	mu      sync.Mutex
	entries map[string]*taskTraceTeamAccessCacheEntry
	now     func() time.Time
}

type taskTraceTeamAccessCacheEntry struct {
	members []string
	err     error
	expires time.Time
	ready   chan struct{}
}

var taskTraceWindowsAccessCache taskTraceTeamAccessCache

func taskTraceTeamAccessCacheKey(root string) string {
	absolute, err := filepath.Abs(root)
	if err == nil {
		root = absolute
	}
	return strings.ToLower(filepath.Clean(root))
}

func taskTraceTeamFilterAccessMembers(members []string) []string {
	result := make([]string, 0, len(members))
	seen := make(map[string]bool, len(members))
	for _, member := range members {
		member = strings.TrimSpace(member)
		key := strings.ToLower(strings.ReplaceAll(member, "/", `\`))
		short := key
		if separator := strings.LastIndex(short, `\`); separator >= 0 {
			short = short[separator+1:]
		}
		if member == "" || strings.HasPrefix(key, `builtin\`) || strings.HasPrefix(key, `nt authority\`) || strings.HasPrefix(key, `creator owner\`) ||
			short == "administrators" || short == "users" || short == "guests" || short == "power users" ||
			short == "管理员" || short == "用户" || short == "来宾" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, member)
	}
	return result
}

func (cache *taskTraceTeamAccessCache) currentTime() time.Time {
	if cache.now != nil {
		return cache.now()
	}
	return time.Now()
}

func (cache *taskTraceTeamAccessCache) read(root string, load func() ([]string, error)) ([]string, error) {
	key := taskTraceTeamAccessCacheKey(root)
	for {
		cache.mu.Lock()
		if entry := cache.entries[key]; entry != nil {
			if entry.ready != nil {
				ready := entry.ready
				cache.mu.Unlock()
				<-ready
				continue
			}
			if cache.currentTime().Before(entry.expires) {
				members, err := slices.Clone(entry.members), entry.err
				cache.mu.Unlock()
				return members, err
			}
		}
		ready := make(chan struct{})
		entry := &taskTraceTeamAccessCacheEntry{ready: ready}
		if cache.entries == nil {
			cache.entries = make(map[string]*taskTraceTeamAccessCacheEntry)
		}
		cache.entries[key] = entry
		cache.mu.Unlock()

		// Do not hold the mutex during PowerShell/SMB access. Readers for this same
		// folder join the scan; a different folder can still be queried immediately.
		members, err := load()
		lifetime := 30 * time.Second
		if err != nil {
			lifetime = 2 * time.Second
		}
		cache.mu.Lock()
		if cache.entries[key] != entry {
			// Permission changes invalidate scans already in progress as well as cached
			// values. Retry instead of publishing the pre-change list after a mutation.
			close(ready)
			cache.mu.Unlock()
			continue
		}
		entry.members, entry.err = slices.Clone(members), err
		entry.expires, entry.ready = cache.currentTime().Add(lifetime), nil
		close(ready)
		cache.mu.Unlock()
		return slices.Clone(members), err
	}
}

func (cache *taskTraceTeamAccessCache) cached(root string) ([]string, bool) {
	key := taskTraceTeamAccessCacheKey(root)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry := cache.entries[key]
	if entry == nil || entry.ready != nil || entry.err != nil || !cache.currentTime().Before(entry.expires) {
		return nil, false
	}
	return slices.Clone(entry.members), true
}

func (cache *taskTraceTeamAccessCache) invalidate(root string) {
	cache.mu.Lock()
	delete(cache.entries, taskTraceTeamAccessCacheKey(root))
	cache.mu.Unlock()
}
