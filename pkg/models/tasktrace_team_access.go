// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import "errors"

// TaskTraceTeamAccessOptions carries explicit confirmation for Windows sharing
// and one-operation elevation. Neither is inferred from a task membership edit.
type TaskTraceTeamAccessOptions struct {
	CreateShare bool
	Elevate     bool
}

var (
	//nolint:gosmopolitan // This actionable message is displayed by the local Chinese permission UI.
	errTaskTraceTeamAdminRequired = errors.New("设置 teamData 共享读写权限需要 Windows 管理员授权")
	errTaskTraceTeamShareRequired = errors.New("windows file sharing confirmation required")
)

func IsTaskTraceTeamAdminRequired(err error) bool {
	return errors.Is(err, errTaskTraceTeamAdminRequired)
}

func IsTaskTraceTeamShareRequired(err error) bool {
	return errors.Is(err, errTaskTraceTeamShareRequired)
}
