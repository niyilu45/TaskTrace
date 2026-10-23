// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package handler

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"xorm.io/builder"
	"xorm.io/xorm"
)

type mutationLockUser struct {
	ID   int64 `xorm:"pk 'id'"`
	Name string
}

func (*mutationLockUser) TableName() string { return "users" }

func TestSQLiteMutationLock(t *testing.T) {
	engine, err := xorm.NewEngine("sqlite3", filepath.Join(t.TempDir(), "concurrent.db")+"?_journal_mode=WAL&_busy_timeout=2000")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	require.NoError(t, engine.Sync2(new(mutationLockUser)))
	_, err = engine.Insert(&mutationLockUser{
		ID:   1,
		Name: "original",
	})
	require.NoError(t, err)

	t.Run("deferred read cannot upgrade after a concurrent commit", func(t *testing.T) {
		first := engine.NewSession()
		defer first.Close()
		require.NoError(t, first.Begin())
		var before mutationLockUser
		found, readErr := first.ID(1).Get(&before)
		require.NoError(t, readErr)
		require.True(t, found)
		_, updateErr := engine.ID(1).Cols("name").Update(&mutationLockUser{Name: "concurrent"})
		require.NoError(t, updateErr)
		_, updateErr = first.ID(1).Cols("name").Update(&mutationLockUser{Name: "stale"})
		require.ErrorContains(t, updateErr, "database is locked")
		require.NoError(t, first.Rollback())
	})

	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{
			false: "commit",
			true:  "rollback",
		}[rollback], func(t *testing.T) {
			first := engine.NewSession()
			defer first.Close()
			require.NoError(t, first.Begin())
			require.NoError(t, lockSQLiteMutation(first))
			var before mutationLockUser
			found, readErr := first.ID(1).Get(&before)
			require.NoError(t, readErr)
			require.True(t, found)

			// WAL readers keep working while this writer is reserved; the lock
			// itself must not change user data or create an ID-zero row.
			var outside mutationLockUser
			found, readErr = engine.ID(1).Get(&outside)
			require.NoError(t, readErr)
			require.True(t, found)
			require.Equal(t, before, outside)
			count, countErr := engine.Where(builder.In("id", []int64{0})).Count(new(mutationLockUser))
			require.NoError(t, countErr)
			require.Zero(t, count)

			if !rollback {
				_, err = first.ID(1).Cols("name").Update(&mutationLockUser{Name: "committed"})
				require.NoError(t, err)
			}
			expected := before.Name
			if !rollback {
				expected = "committed"
			}
			started := make(chan struct{})
			result := make(chan error, 1)
			observed := make(chan string, 1)
			go func() {
				close(started)
				second := engine.NewSession()
				defer second.Close()
				if beginErr := second.Begin(); beginErr != nil {
					result <- beginErr
					return
				}
				defer second.Rollback()
				if lockErr := lockSQLiteMutation(second); lockErr != nil {
					result <- lockErr
					return
				}
				var latest mutationLockUser
				if _, getErr := second.ID(1).Get(&latest); getErr != nil {
					result <- getErr
					return
				}
				observed <- latest.Name
				if _, updateErr := second.ID(1).Cols("name").Update(&mutationLockUser{Name: "second"}); updateErr != nil {
					result <- updateErr
					return
				}
				result <- second.Commit()
			}()
			<-started
			select {
			case early := <-result:
				t.Fatalf("second mutation did not wait for the first transaction: %v", early)
			case <-time.After(40 * time.Millisecond):
			}
			if rollback {
				require.NoError(t, first.Rollback())
			} else {
				require.NoError(t, first.Commit())
			}
			select {
			case writeErr := <-result:
				require.NoError(t, writeErr)
			case <-time.After(3 * time.Second):
				t.Fatal("waiting writer did not resume")
			}
			require.Equal(t, expected, <-observed, "waiting transaction read a stale snapshot")
			var saved mutationLockUser
			found, readErr = engine.ID(1).Get(&saved)
			require.NoError(t, readErr)
			require.True(t, found)
			require.Equal(t, "second", saved.Name)
		})
	}
}
