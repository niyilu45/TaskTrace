// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"fmt"
	"regexp"
	"strings"
)

var taskTraceTeamConflictAttachment = regexp.MustCompile(`tasktrace-team-attachment:([a-zA-Z0-9-]+)`)

// Presentation must not change conflict IDs or the canonical values accepted by Resolve.
// Use the existing local mapping without reading or writing the network share again.
func taskTraceTeamConflictPreviews(binding *TaskTraceTeamBinding) []TaskTraceTeamConflict {
	result := make([]TaskTraceTeamConflict, len(binding.Conflicts))
	for i, conflict := range binding.Conflicts {
		preview := func(value string) string {
			return taskTraceTeamConflictAttachment.ReplaceAllStringFunc(value, func(source string) string {
				id := strings.TrimPrefix(source, "tasktrace-team-attachment:")
				localID := binding.LocalAttachments[taskTraceTeamLocalAttachmentKey(conflict.NodeID, id)]
				taskID := binding.NodeTasks[conflict.NodeID]
				if localID == 0 || taskID == 0 {
					return source
				}
				return fmt.Sprintf("/api/v1/tasks/%d/attachments/%d", taskID, localID)
			})
		}
		result[i] = conflict
		result[i].BasePreview = preview(conflict.Base)
		result[i].Options = append([]TaskTraceTeamConflictOption(nil), conflict.Options...)
		for j := range result[i].Options {
			result[i].Options[j].Preview = preview(result[i].Options[j].Value)
		}
	}
	return result
}
