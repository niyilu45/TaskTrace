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
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func taskTraceTeamFieldReceipt(snapshot TaskTraceTeamSnapshot, task TaskTraceTeamTask, field string) (key, fingerprint string) {
	identity := sha256.Sum256([]byte(strings.Join([]string{strings.ToLower(snapshot.Actor), snapshot.DeviceID, task.NodeID, field}, "\x00")))
	base, exists := snapshot.Base[task.NodeID]
	baseValue := "missing"
	if exists {
		baseValue = "present:" + taskTraceTeamCanonicalFieldValue(task, field, taskTraceTeamBaseValue(base, field))
	}
	value := taskTraceTeamCanonicalFieldValue(task, field, taskTraceTeamFieldValue(task, field))
	digest := sha256.Sum256([]byte(baseValue + "\x00" + value))
	return hex.EncodeToString(identity[:]), hex.EncodeToString(digest[:])
}

func taskTraceTeamAcknowledgeSnapshots(snapshots []TaskTraceTeamSnapshot, manifest *TaskTraceTeamManifest) []TaskTraceTeamSnapshot {
	result := make([]TaskTraceTeamSnapshot, 0, len(snapshots))
	for _, original := range snapshots {
		snapshot := original
		snapshot.Base = taskTraceTeamCopyBase(original.Base)
		for _, task := range original.Tasks {
			fields := []string{"title", "description", "status"}
			ids := map[string]bool{}
			for _, body := range []string{task.Outstanding, original.Base[task.NodeID].Outstanding, manifest.Base[task.NodeID].Outstanding} {
				items, _ := taskTraceTeamOutstandingItems(body)
				for id := range items {
					ids[id] = true
				}
			}
			for id := range ids {
				fields = append(fields, "outstanding:"+id)
			}
			for _, field := range fields {
				key, fingerprint := taskTraceTeamFieldReceipt(original, task, field)
				if manifest.AcceptedFields[key] != fingerprint {
					continue
				}
				base, exists := snapshot.Base[task.NodeID]
				if !exists {
					base = manifest.Base[task.NodeID]
				}
				taskTraceTeamSetBaseValue(&base, field, taskTraceTeamFieldValue(task, field))
				snapshot.Base[task.NodeID] = base
			}
		}
		result = append(result, snapshot)
	}
	return result
}

func taskTraceTeamRememberAcceptedField(manifest *TaskTraceTeamManifest, snapshots []TaskTraceTeamSnapshot, node, field, accepted string) bool {
	changed := false
	for _, snapshot := range snapshots {
		for _, task := range snapshot.Tasks {
			if task.NodeID != node || taskTraceTeamCanonicalFieldValue(task, field, taskTraceTeamFieldValue(task, field)) != accepted {
				continue
			}
			key, fingerprint := taskTraceTeamFieldReceipt(snapshot, task, field)
			if manifest.AcceptedFields[key] == fingerprint {
				continue
			}
			if manifest.AcceptedFields == nil {
				manifest.AcceptedFields = map[string]string{}
			}
			manifest.AcceptedFields[key] = fingerprint
			changed = true
		}
	}
	return changed
}
