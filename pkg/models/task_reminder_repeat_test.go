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

package models

import (
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

const weeklyRuleFixture = `{"weekdays":[1,3,5],"startDate":"2026-10-12","endDate":"2026-10-23","time":"09:30"}`

func TestWeeklyReminderRule(t *testing.T) {
	rule, err := parseReminderRepeatRule(weeklyRuleFixture)
	require.NoError(t, err)
	zone := time.FixedZone("CST", 8*3600)
	for _, row := range []struct {
		date string
		ok   bool
	}{{"2026-10-12", true}, {"2026-10-14", true}, {"2026-10-23", true}, {"2026-10-13", false}, {"2026-10-09", false}, {"2026-10-26", false}} {
		day, err := time.ParseInLocation("2006-01-02", row.date, zone)
		require.NoError(t, err)
		due, ok := rule.onDate(day, zone)
		require.Equal(t, row.ok, ok, row.date)
		if ok {
			require.Equal(t, "09:30", due.Format("15:04"))
			require.Equal(t, "01:30", due.UTC().Format("15:04"))
		}
	}
	for _, raw := range []string{`{}`, `{"weekdays":[],"startDate":"2026-10-12","endDate":"2026-10-23","time":"09:30"}`, `{"weekdays":[7],"startDate":"2026-10-12","endDate":"2026-10-23","time":"09:30"}`, `{"weekdays":[1],"startDate":"2026-10-13","endDate":"2026-10-13","time":"09:30"}`, `{"weekdays":[1],"startDate":"2026-10-23","endDate":"2026-10-12","time":"09:30"}`, `{"weekdays":[1],"startDate":"2026-02-30","endDate":"2026-10-23","time":"09:30"}`} {
		_, err = parseReminderRepeatRule(raw)
		require.Error(t, err, raw)
	}
}
func TestWeeklyReminderPersistence(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	task := &Task{ID: 1}
	require.NoError(t, task.ReadOne(s, &user.User{ID: 1}))
	first := time.Date(2026, 10, 12, 1, 30, 0, 0, time.UTC)
	task.Reminders = []*TaskReminder{{Reminder: first}, {Reminder: first, RepeatRule: weeklyRuleFixture}}
	require.NoError(t, task.updateReminders(s, task))
	reminders, err := getRemindersForTasks(s, []int64{1})
	require.NoError(t, err)
	require.Len(t, reminders, 2, "single and weekly at same instant are distinct")
	require.Equal(t, 1, func() int {
		count := 0
		for _, r := range reminders {
			if r.RepeatRule != "" {
				count++
			}
		}
		return count
	}())
	task.Reminders = []*TaskReminder{{Reminder: first, RepeatRule: weeklyRuleFixture}, {Reminder: first, RepeatRule: weeklyRuleFixture}}
	require.NoError(t, task.updateReminders(s, task))
	require.Len(t, task.Reminders, 1, "identical rules are deduplicated")
	require.NoError(t, s.Commit())
}
func TestWeeklyReminderOutstandingRoundTrip(t *testing.T) {
	body := `<h3 data-tasktrace-comment-type="outstanding">list</h3><ul><li data-id="one" data-reminder="2026-10-12T01:30:00Z" data-reminder-repeat="{&quot;weekdays&quot;:[1,3,5],&quot;startDate&quot;:&quot;2026-10-12&quot;,&quot;endDate&quot;:&quot;2026-10-23&quot;,&quot;time&quot;:&quot;09:30&quot;}">item</li></ul>`
	items, order := taskTraceTeamOutstandingItems(body)
	restored := taskTraceTeamOutstandingHTML(items, order)
	_, nodes := taskTraceTeamOutstandingNodes(restored)
	require.Len(t, nodes, 1)
	require.JSONEq(t, weeklyRuleFixture, taskTraceAttribute(nodes[0], "data-reminder-repeat"))
	require.Contains(t, taskTraceOutstandingMetadata(nodes[0]), "data-reminder-repeat")
}

func TestWeeklyReminderNotifications(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	_, err := s.ID(1).Cols("timezone").Update(&user.User{Timezone: "UTC"})
	require.NoError(t, err)
	task := &Task{ID: 1}
	require.NoError(t, task.ReadOne(s, &user.User{ID: 1}))
	task.Reminders = []*TaskReminder{{Reminder: time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC), RepeatRule: weeklyRuleFixture}}
	require.NoError(t, task.updateReminders(s, task))
	now := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	due, err := getTasksWithRemindersDueAndTheirUsers(s, now, nil)
	require.NoError(t, err)
	found := false
	for _, notification := range due {
		if notification.User.ID == 1 && notification.Task.ID == 1 {
			found = true
			require.True(t, notification.TaskReminder.Reminder.Equal(now))
		}
	}
	require.True(t, found, "weekly occurrence after the original reminder date must be delivered")
	due, err = getTasksWithRemindersDueAndTheirUsers(s, now.AddDate(0, 0, 1), nil)
	require.NoError(t, err)
	for _, notification := range due {
		require.NotEqual(t, int64(1), notification.Task.ID, "unselected day")
	}
}
