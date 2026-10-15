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
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"code.vikunja.io/api/pkg/web"
)

// Calendar dates are stored in a JSON rule, separate from absolute timestamps.
// Weekdays follow time.Weekday / Date.getDay / DayOfWeek (Sunday = 0).
type reminderRepeatRule struct {
	Weekdays  []int  `json:"weekdays"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Time      string `json:"time"`
}
type errReminderRepeat string

func (e errReminderRepeat) Error() string { return string(e) }
func (e errReminderRepeat) HTTPError() web.HTTPError {
	return web.HTTPError{HTTPCode: http.StatusBadRequest, Code: 1045, Message: string(e)}
}
func parseReminderRepeatRule(raw string) (*reminderRepeatRule, error) {
	rule := &reminderRepeatRule{}
	if len(raw) > 1024 || json.Unmarshal([]byte(raw), rule) != nil {
		return nil, errReminderRepeat("Invalid weekly reminder rule")
	}
	start, err := time.Parse("2006-01-02", rule.StartDate)
	if err != nil {
		return nil, errReminderRepeat("Select a valid reminder start date")
	}
	end, err := time.Parse("2006-01-02", rule.EndDate)
	if err != nil || end.Before(start) {
		return nil, errReminderRepeat("The reminder end date must not precede its start date")
	}
	if _, err = time.Parse("15:04", rule.Time); err != nil || len(rule.Time) != 5 {
		return nil, errReminderRepeat("Select a valid reminder time")
	}
	days := map[int]bool{}
	for _, day := range rule.Weekdays {
		if day < 0 || day > 6 {
			return nil, errReminderRepeat("Reminder weekdays must be between 0 and 6")
		}
		days[day] = true
	}
	if len(days) == 0 {
		return nil, errReminderRepeat("Select at least one reminder weekday")
	}
	rule.Weekdays = nil
	for day := range days {
		rule.Weekdays = append(rule.Weekdays, day)
	}
	sort.Ints(rule.Weekdays)
	for date := start; !date.After(end) && date.Before(start.AddDate(0, 0, 7)); date = date.AddDate(0, 0, 1) {
		if days[int(date.Weekday())] {
			return rule, nil
		}
	}
	return nil, errReminderRepeat("The reminder date range contains none of the selected weekdays")
}
func (r *reminderRepeatRule) canonical() (string, error) {
	data, err := json.Marshal(r)
	return string(data), err
}
func (r *reminderRepeatRule) onDate(date time.Time, location *time.Location) (time.Time, bool) {
	key := date.Format("2006-01-02")
	if key < r.StartDate || key > r.EndDate {
		return time.Time{}, false
	}
	found := false
	for _, day := range r.Weekdays {
		if day == int(date.Weekday()) {
			found = true
			break
		}
	}
	if !found {
		return time.Time{}, false
	}
	due, err := time.ParseInLocation("2006-01-02 15:04", key+" "+r.Time, location)
	return due, err == nil
}
