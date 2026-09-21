// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web"

	"github.com/danielgtaylor/huma/v2"
	"xorm.io/xorm"
)

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
		OperationID: "tasktrace-team-notifications-read", Summary: "Dismiss team collaboration notifications",
		Description: "Removes selected notification files for the current Windows username. An empty list dismisses all visible team notifications.",
		Method:      http.MethodPost, Path: "/tasktrace/team/notifications/read", Tags: tags,
	}, taskTraceTeamNotificationsRead)
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

func taskTraceTeamStatus(ctx context.Context, _ *struct{}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamReadSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamReadStatus(s, a)
	if err != nil {
		return nil, huma.Error500InternalServerError("read team collaboration status", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: &status}, nil
}

func taskTraceTeamShare(ctx context.Context, in *struct {
	TaskID int64 `path:"task"`
	Body   struct {
		Members []string `json:"members" minItems:"1" doc:"Windows usernames allowed to collaborate."`
	}
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamShare(s, a, models.TaskTraceTeamShareRequest{TaskID: in.TaskID, Members: in.Body.Members})
	if err != nil {
		return nil, translateDomainError(err)
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
	Body models.TaskTraceTeamPermissionsRequest
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamConfigurePermissions(s, a, in.Body)
	if err != nil {
		return nil, translateDomainError(err)
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
		return nil, huma.Error500InternalServerError("dismiss team notifications", err)
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
	Body    models.TaskTraceTeamMemberAccessRequest
	Elevate bool `header:"X-TaskTrace-Elevate" doc:"Retry this permission change with a one-time Windows administrator prompt."`
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamGrantMemberAccess(s, a, in.Body, in.Elevate)
	if err != nil {
		if models.IsTaskTraceTeamAdminRequired(err) {
			return nil, huma.Error403Forbidden("Windows administrator authorization required", err)
		}
		return nil, huma.Error422UnprocessableEntity("grant teamData access", err)
	}
	if err := s.Commit(); err != nil {
		return nil, huma.Error500InternalServerError("save teamData access", err)
	}
	return &singleBody[models.TaskTraceTeamStatus]{Body: status}, nil
}

func taskTraceTeamMembersAccessDelete(ctx context.Context, in *struct {
	Member string `path:"member" doc:"The Windows account whose unassigned access should be removed."`
}) (*singleBody[models.TaskTraceTeamStatus], error) {
	s, a, err := taskTraceTeamWriteSession(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	status, err := models.TaskTraceTeamRemoveMemberAccess(s, a, in.Member)
	if err != nil {
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
