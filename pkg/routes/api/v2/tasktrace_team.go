// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"errors"
	"net/http"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web"

	"github.com/danielgtaylor/huma/v2"
	"xorm.io/xorm"
)

func taskTraceTeamMutationError(message string, err error) error {
	if models.IsTaskTraceTeamShareRequired(err) {
		return huma.Error409Conflict("Windows file sharing confirmation required", err)
	}
	if models.IsTaskTraceTeamAdminRequired(err) {
		return huma.Error403Forbidden("Windows administrator authorization required", err)
	}
	translated := translateDomainError(err)
	var statusError huma.StatusError
	if errors.As(translated, &statusError) {
		return translated
	}
	return huma.Error422UnprocessableEntity(message, err)
}

func init() { AddRouteRegistrar(RegisterTaskTraceTeamRoutes) }

func RegisterTaskTraceTeamRoutes(api huma.API) {
	tags := []string{"tasktrace-team"}
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-status", Summary: "Read local team collaboration status",
		Description: "Returns the Windows shared-folder repository, candidate members, local shared-task mappings, pending notifications, and conflicts.",
		Method:      http.MethodGet, Path: "/tasktrace/team", Tags: tags,
	}, taskTraceTeamStatus)
	Register(api, huma.Operation{
		OperationID: "tasks-team-share", Summary: "Share a task subtree with LAN members",
		Description: "Creates a teamData repository entry for this task and all its descendants. Parent tasks and unshared siblings are excluded. Priority is never exported.",
		Method:      http.MethodPost, Path: "/tasks/{task}/team/share", Tags: tags,
	}, taskTraceTeamShare)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-import", Summary: "Import a TaskTrace team task link",
		Description: "Imports only the linked task subtree into the selected local project. Its local parent and priority remain personal settings.",
		Method:      http.MethodPost, Path: "/tasktrace/team/import", Tags: tags,
	}, taskTraceTeamImport)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-sync", Summary: "Synchronize every team task",
		Description: "Writes this member's snapshot, merges comments and daily progress, and returns all metadata conflicts together for resolution.",
		Method:      http.MethodPost, Path: "/tasktrace/team/sync", Tags: tags,
	}, taskTraceTeamSync)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-configure", Summary: "Configure a local team task",
		Description: "Changes per-machine collaboration behavior. Notification defaults to enabled and priority is always local.",
		Method:      http.MethodPut, Path: "/tasktrace/team/configure", Tags: tags,
	}, taskTraceTeamConfigure)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-permissions-configure", Summary: "Configure collaboration permissions",
		Description: "Replaces member read and write choices for one shared task or outstanding item. Owner and assignee permissions remain read-write and cannot be reduced.",
		Method:      http.MethodPut, Path: "/tasktrace/team/permissions", Tags: tags,
	}, taskTraceTeamPermissionsConfigure)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-resolve", Summary: "Resolve team task conflicts",
		Description: "Applies one selected value for each supplied task-name, completion, status, or outstanding-item conflict.",
		Method:      http.MethodPost, Path: "/tasktrace/team/conflicts/resolve", Tags: tags,
	}, taskTraceTeamResolve)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-notifications-read", Summary: "Mark team collaboration notifications as read",
		Description: "Records personal read receipts without removing shared notifications. An empty list marks all existing notifications read; future notifications remain unread.",
		Method:      http.MethodPost, Path: "/tasktrace/team/notifications/read", Tags: tags,
	}, taskTraceTeamNotificationsRead)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-notifications-delete", Summary: "Delete team collaboration notifications",
		Description: "Explicitly deletes selected notifications for the current user. An empty list deletes all existing notifications; other members are unaffected.",
		Method:      http.MethodPost, Path: "/tasktrace/team/notifications/delete", Tags: tags,
		DefaultStatus: http.StatusOK,
	}, taskTraceTeamNotificationsDelete)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-members-search", Summary: "Search Windows collaboration accounts",
		Description: "Returns matching local or domain Windows accounts while the user types a team member name.",
		Method:      http.MethodGet, Path: "/tasktrace/team/members/search", Tags: tags,
	}, taskTraceTeamMembersSearch)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-members-access-create", Summary: "Grant teamData access to a member",
		Description: "Grants the selected Windows account read/write access to both the teamData folder and its SMB share.",
		Method:      http.MethodPost, Path: "/tasktrace/team/members/access", Tags: tags,
	}, taskTraceTeamMembersAccessCreate)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-members-access-delete", Summary: "Remove unassigned teamData access",
		Description: "Removes folder and SMB access only when the account does not belong to any local collaboration team.",
		Method:      http.MethodDelete, Path: "/tasktrace/team/members/access/{member}", Tags: tags, DefaultStatus: http.StatusOK,
	}, taskTraceTeamMembersAccessDelete)
	Register(api, huma.Operation{
		OperationID: "tasktrace-team-members-import", Summary: "Import members from a team link",
		Description: "Grants access to every valid member, skips the current user, and reports individual failures without rolling back successful members.",
		Method:      http.MethodPost, Path: "/tasktrace/team/members/import", Tags: tags,
	}, taskTraceTeamMembersImport)
}

func taskTraceTeamReadSession(ctx context.Context) (*xorm.Session, web.Auth, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, nil, err
	}
	return db.NewReadSession(), a, nil
}

func taskTraceTeamWriteSession(ctx context.Context) (*xorm.Session, web.Auth, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, nil, err
	}
	return db.NewSession(), a, nil
}

func taskTraceTeamStatus(ctx context.Context, in *struct {
	RefreshAccess bool `header:"X-TaskTrace-Refresh-Access" doc:"Rescan current Windows share permissions when opening member management instead of reusing the polling cache."`
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamReadSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	//nolint:contextcheck // A shared, bounded Windows metadata scan may also serve other waiting status reads.
	status, err := models.TaskTraceTeamReadStatus(s, a, in.RefreshAccess)
	if err != nil {
		return nil, huma.Error500InternalServerError("read team collaboration status", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: &status}, nil
}

func taskTraceTeamShare(ctx context.Context, in *struct {
	TaskID      int64 `path:"task"`
	Elevate     bool  `header:"X-TaskTrace-Elevate" doc:"Consent to a one-time Windows administrator prompt for this permission change."`
	CreateShare bool  `header:"X-TaskTrace-Create-Share" doc:"Consent to create a restricted Windows share for teamData when none exists."`
	Body        struct {
		Members []string `json:"members" minItems:"1" doc:"Windows usernames allowed to collaborate."`
	}
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	// Windows ACL changes must finish their own bounded operation after consent; a disconnected browser must not interrupt rollback.
	//nolint:contextcheck // The model serializes Windows access changes using an independent timeout.
	status, err := models.TaskTraceTeamShare(s, a, models.TaskTraceTeamShareRequest{TaskID: in.TaskID, Members: in.Body.Members, Elevate: in.Elevate, CreateShare: in.CreateShare})
	if err != nil {
		return nil, taskTraceTeamMutationError("无法更新协作成员", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save team task", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamImport(ctx context.Context, in *struct {
	Body models.TaskTraceTeamImportRequest
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamImport(s, a, in.Body)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("cannot import team task", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save imported team task", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamSync(ctx context.Context, _ *struct{}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamSync(s, a)
	if err != nil {
		return nil, huma.Error500InternalServerError("synchronize team tasks", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save synchronized team tasks", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamConfigure(ctx context.Context, in *struct {
	Body models.TaskTraceTeamConfigureRequest
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamConfigure(s, a, in.Body)
	if err != nil {
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save team task settings", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamPermissionsConfigure(ctx context.Context, in *struct {
	Body        models.TaskTraceTeamPermissionsRequest
	Elevate     bool `header:"X-TaskTrace-Elevate" doc:"Consent to a one-time Windows administrator prompt for this permission change."`
	CreateShare bool `header:"X-TaskTrace-Create-Share" doc:"Consent to create a restricted Windows share for teamData when none exists."`
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	in.Body.Elevate = in.Elevate
	in.Body.CreateShare = in.CreateShare
	status, err := models.TaskTraceTeamConfigurePermissions(s, a, in.Body)
	if err != nil {
		return nil, taskTraceTeamMutationError("无法更新协作权限或受理人", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save team task permissions", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}
func taskTraceTeamResolve(ctx context.Context, in *struct {
	Body models.TaskTraceTeamResolveRequest
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamResolve(s, a, in.Body)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("cannot resolve team task conflicts", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save conflict resolutions", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamNotificationsRead(ctx context.Context, in *struct {
	Body models.TaskTraceTeamNotificationsReadRequest
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamNotificationsRead(s, a, in.Body)
	if err != nil {
		return nil, taskTraceTeamMutationError("mark team notifications as read", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save notification state", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamNotificationsDelete(ctx context.Context, in *struct {
	Body models.TaskTraceTeamNotificationsDeleteRequest
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	//nolint:contextcheck // The fast status path uses cached Windows access; it never launches PowerShell.
	status, err := models.TaskTraceTeamNotificationsDelete(s, a, in.Body)
	if err != nil {
		return nil, taskTraceTeamMutationError("delete team notifications", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save notification state", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamMembersSearch(ctx context.Context, in *struct {
	Query string `query:"q" doc:"A partial Windows username, account name, or display name."`
	Quick bool   `header:"X-TaskTrace-Quick" doc:"Return direct and local matches without waiting for domain search."`
}) (*singleBody[models.TaskTraceTeamMemberSearchResult], error) {
	s, _, err := taskTraceTeamReadSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	result, err := models.TaskTraceTeamSearchMembers(ctx, in.Query, in.Quick)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("search Windows accounts", err)
	}
	return &singleBody[models.TaskTraceTeamMemberSearchResult]{Body: &result}, nil
}

func taskTraceTeamMembersAccessCreate(ctx context.Context, in *struct {
	Body        models.TaskTraceTeamMemberAccessRequest
	CreateShare bool `header:"X-TaskTrace-Create-Share" doc:"Consent to create a restricted Windows share for teamData when none exists."`
	Elevate     bool `header:"X-TaskTrace-Elevate" doc:"Retry this permission change with a one-time Windows administrator prompt."`
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	//nolint:contextcheck // Complete the bounded OS permission change even if the requesting browser disconnects.
	status, err := models.TaskTraceTeamGrantMemberAccessWithOptions(s, a, in.Body, models.TaskTraceTeamAccessOptions{CreateShare: in.CreateShare, Elevate: in.Elevate})
	if err != nil {
		return nil, taskTraceTeamMutationError("grant teamData access", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save teamData access", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamMembersAccessDelete(ctx context.Context, in *struct {
	Member  string `path:"member" doc:"The Windows account whose unassigned access should be removed."`
	Elevate bool   `header:"X-TaskTrace-Elevate" doc:"Retry this permission change with a one-time Windows administrator prompt."`
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamRemoveMemberAccess(s, a, in.Member, in.Elevate)
	if err != nil {
		if models.IsTaskTraceTeamAdminRequired(err) {
			return nil, huma.Error403Forbidden("Windows administrator authorization required", err)
		}
		return nil, huma.Error422UnprocessableEntity("remove teamData access", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save teamData access", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamMembersImport(ctx context.Context, in *struct {
	Body models.TaskTraceTeamMemberImportRequest
}) (*singleBody[models.TaskTraceTeamMemberImportResult], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	result, err := models.TaskTraceTeamImportMembers(s, a, in.Body)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("import team members", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save imported team members", err)
	}
	return &singleBody[models.TaskTraceTeamMemberImportResult]{Body: result}, nil
}
