// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/modules/avatar"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"github.com/google/uuid"
	"xorm.io/xorm"
)

const taskTraceTeamSchema = 1

var taskTraceTeamMu sync.Mutex

type TaskTraceTeamRepositoryInfo struct {
	Path       string   `json:"path"`
	Paths      []string `json:"-"`
	Computer   string   `json:"computer,omitempty"`
	Candidates []string `json:"candidates"`
	Shared     bool     `json:"shared"`
}

type TaskTraceTeamManifest struct {
	Schema      int                                        `json:"schema"`
	ShareID     string                                     `json:"share_id"`
	RootNode    string                                     `json:"root_node"`
	Owner       string                                     `json:"owner"`
	Members     []string                                   `json:"members"`
	Permissions map[string][]TaskTraceTeamMemberPermission `json:"permissions,omitempty"`
	TokenHash   string                                     `json:"token_hash"`
	Created     time.Time                                  `json:"created"`
	Updated     time.Time                                  `json:"updated"`
}

type TaskTraceTeamComment struct {
	ID      string    `json:"id"`
	Body    string    `json:"body"`
	Author  string    `json:"author"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

type TaskTraceTeamAttachment struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Mime               string `json:"mime"`
	Size               uint64 `json:"size"`
	SourceTaskID       int64  `json:"source_task_id"`
	SourceAttachmentID int64  `json:"source_attachment_id"`
}

type TaskTraceTeamTask struct {
	NodeID      string                    `json:"node_id"`
	ParentNode  string                    `json:"parent_node,omitempty"`
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	Owner       string                    `json:"owner,omitempty"`
	Assignees   []string                  `json:"assignees,omitempty"`
	Done        bool                      `json:"done"`
	Status      TaskStatus                `json:"status"`
	Outstanding string                    `json:"outstanding,omitempty"`
	Comments    []TaskTraceTeamComment    `json:"comments"`
	Attachments []TaskTraceTeamAttachment `json:"attachments,omitempty"`
	Updated     time.Time                 `json:"updated"`
}

type TaskTraceTeamSnapshot struct {
	Schema         int                 `json:"schema"`
	ShareID        string              `json:"share_id"`
	Actor          string              `json:"actor"`
	DeviceID       string              `json:"device_id"`
	Updated        time.Time           `json:"updated"`
	Avatar         string              `json:"avatar,omitempty"`
	ResolutionAcks map[string]string   `json:"resolution_acks,omitempty"`
	Tasks          []TaskTraceTeamTask `json:"tasks"`
}

type TaskTraceTeamBase struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Done        bool       `json:"done"`
	Status      TaskStatus `json:"status"`
	Outstanding string     `json:"outstanding"`
}

type TaskTraceTeamConflictOption struct {
	Author string `json:"author"`
	Value  string `json:"value"`
}

type TaskTraceTeamConflict struct {
	ID        string                        `json:"id"`
	ShareID   string                        `json:"share_id"`
	NodeID    string                        `json:"node_id"`
	TaskID    int64                         `json:"task_id"`
	TaskTitle string                        `json:"task_title"`
	Field     string                        `json:"field"`
	Base      string                        `json:"base"`
	Options   []TaskTraceTeamConflictOption `json:"options"`
}

type TaskTraceTeamBinding struct {
	ShareID          string                       `json:"share_id"`
	Repository       string                       `json:"repository"`
	Secret           string                       `json:"secret"`
	Owner            string                       `json:"owner"`
	Members          []string                     `json:"members"`
	RootTaskID       int64                        `json:"root_task_id"`
	ProjectID        int64                        `json:"project_id"`
	NodeTasks        map[string]int64             `json:"node_tasks"`
	Base             map[string]TaskTraceTeamBase `json:"base"`
	Notify           bool                         `json:"notify"`
	LastSnapshotHash string                       `json:"last_snapshot_hash,omitempty"`
	LastSync         time.Time                    `json:"last_sync,omitempty"`
	LastError        string                       `json:"last_error,omitempty"`
	Conflicts        []TaskTraceTeamConflict      `json:"conflicts,omitempty"`
	ResolutionAcks   map[string]string            `json:"resolution_acks,omitempty"`
	LocalAttachments map[string]int64             `json:"local_attachments,omitempty"`
}

type taskTraceTeamResolutionRecord struct {
	ID      string    `json:"id"`
	NodeID  string    `json:"node_id"`
	Field   string    `json:"field"`
	Value   string    `json:"value"`
	Updated time.Time `json:"updated"`
}

type taskTraceTeamState struct {
	Schema   int                    `json:"schema"`
	DeviceID string                 `json:"device_id"`
	Bindings []TaskTraceTeamBinding `json:"bindings"`
}

type TaskTraceTeamShareRequest struct {
	TaskID  int64    `json:"task_id"`
	Members []string `json:"members"`
}

type TaskTraceTeamImportRequest struct {
	Link      string `json:"link"`
	ProjectID int64  `json:"project_id"`
}

type TaskTraceTeamConfigureRequest struct {
	ShareID string `json:"share_id"`
	Notify  bool   `json:"notify"`
}

type TaskTraceTeamNotificationsReadRequest struct {
	IDs []string `json:"ids"`
}

type TaskTraceTeamResolution struct {
	ConflictID string `json:"conflict_id"`
	Value      string `json:"value"`
}

type TaskTraceTeamResolveRequest struct {
	ShareID     string                    `json:"share_id"`
	Resolutions []TaskTraceTeamResolution `json:"resolutions"`
}

type TaskTraceTeamBindingStatus struct {
	ShareID              string                          `json:"share_id"`
	Owner                string                          `json:"owner"`
	Members              []string                        `json:"members"`
	RootTaskID           int64                           `json:"root_task_id"`
	RootTaskTitle        string                          `json:"root_task_title,omitempty" readOnly:"true" doc:"The local title of the shared task root."`
	TaskIDs              []int64                         `json:"task_ids"`
	Link                 string                          `json:"link"`
	MemberLink           string                          `json:"member_link" readOnly:"true" doc:"A portable link containing the members of this collaboration team."`
	Notify               bool                            `json:"notify"`
	CanManagePermissions bool                            `json:"can_manage_permissions" readOnly:"true" doc:"Whether the current user may change collaboration permissions."`
	PermissionTargets    []TaskTraceTeamPermissionTarget `json:"permission_targets" readOnly:"true" doc:"Task and outstanding-item permissions visible to the current user."`
	LastSync             time.Time                       `json:"last_sync,omitempty"`
	LastError            string                          `json:"last_error,omitempty"`
	Conflicts            []TaskTraceTeamConflict         `json:"conflicts"`
}

type TaskTraceTeamNotification struct {
	ID              string    `json:"id"`
	ShareID         string    `json:"share_id"`
	Actor           string    `json:"actor"`
	Avatar          string    `json:"avatar,omitempty"`
	TaskTitle       string    `json:"task_title"`
	NodeID          string    `json:"node_id,omitempty"`
	SharedCommentID string    `json:"shared_comment_id,omitempty"`
	TaskID          int64     `json:"task_id,omitempty"`
	CommentID       int64     `json:"comment_id,omitempty"`
	Created         time.Time `json:"created"`
}

type TaskTraceTeamMemberProfile struct {
	Username string `json:"username"`
	Avatar   string `json:"avatar,omitempty"`
}

type TaskTraceTeamStatus struct {
	Enabled           bool                         `json:"enabled"`
	Username          string                       `json:"username"`
	Repository        TaskTraceTeamRepositoryInfo  `json:"repository"`
	UnassignedMembers []string                     `json:"unassigned_members" readOnly:"true" doc:"Accounts with teamData read/write access that do not belong to any collaboration team."`
	Bindings          []TaskTraceTeamBindingStatus `json:"bindings"`
	Conflicts         []TaskTraceTeamConflict      `json:"conflicts"`
	Notifications     []TaskTraceTeamNotification  `json:"notifications"`
	Profiles          []TaskTraceTeamMemberProfile `json:"profiles"`
}

type taskTraceTeamLink struct {
	Schema       int      `json:"schema"`
	Repository   string   `json:"repository"`
	Repositories []string `json:"repositories,omitempty"`
	ShareID      string   `json:"share_id"`
	Secret       string   `json:"secret"`
}

var taskTraceTeamMarker = regexp.MustCompile(`<!--tasktrace-team:([A-Za-z0-9_-]+)-->`)
var taskTraceTeamOutstandingItem = regexp.MustCompile(`(?s)<li[^>]*data-id="([^"]+)"[^>]*>(.*?)</li>`)
var taskTraceTeamOutstandingPriority = regexp.MustCompile(`(?i)\sdata-priority="([0-9])"`)

func taskTraceTeamRoot() string {
	if root := strings.TrimSpace(os.Getenv("TASKTRACE_TEAM_ROOT")); root != "" {
		return root
	}
	if root := strings.TrimSpace(os.Getenv("TASKTRACE_PACKAGE_ROOT")); root != "" {
		return filepath.Join(root, "teamData")
	}
	return ""
}

func taskTraceTeamStatePath() string {
	root := strings.TrimSpace(os.Getenv("TASKTRACE_DATA_ROOT"))
	if root == "" {
		return ""
	}
	return filepath.Join(root, "team-sync.json")
}

func taskTraceTeamEnabled() bool { return taskTraceTeamRoot() != "" && taskTraceTeamStatePath() != "" }

func taskTraceTeamReadJSON(path string, value interface{}) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

func taskTraceTeamWriteJSON(path string, value interface{}) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp-" + uuid.NewString()
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func taskTraceTeamLoadState() (taskTraceTeamState, error) {
	state := taskTraceTeamState{Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{}}
	path := taskTraceTeamStatePath()
	if path == "" {
		return state, errors.New("TaskTrace team sync is unavailable")
	}
	if err := taskTraceTeamReadJSON(path, &state); err != nil && !os.IsNotExist(err) {
		return state, err
	}
	if state.DeviceID == "" {
		state.DeviceID = uuid.NewString()
	}
	if state.Schema == 0 {
		state.Schema = taskTraceTeamSchema
	}
	return state, nil
}

func taskTraceTeamSaveState(state taskTraceTeamState) error {
	return taskTraceTeamWriteJSON(taskTraceTeamStatePath(), &state)
}

func taskTraceTeamAppendRepository(paths []string, value string) []string {
	value = strings.TrimRight(strings.Trim(strings.TrimSpace(value), "\"'"), `\/`)
	if value == "" {
		return paths
	}
	for _, existing := range paths {
		if strings.EqualFold(existing, value) {
			return paths
		}
	}
	return append(paths, value)
}

func taskTraceTeamRepositoryInfo(root string) TaskTraceTeamRepositoryInfo {
	info := TaskTraceTeamRepositoryInfo{Path: root, Paths: []string{}, Candidates: []string{}}
	path := filepath.Join(root, "repository-info.json")
	_ = taskTraceTeamReadJSON(path, &info)
	var aliases struct {
		Paths []string `json:"paths"`
	}
	_ = taskTraceTeamReadJSON(path, &aliases)
	info.Paths = aliases.Paths
	if info.Path == "" {
		info.Path = root
	}
	info.Paths = taskTraceTeamAppendRepository(info.Paths, info.Path)
	if candidates, err := taskTraceTeamListWindowsAccess(root); err == nil {
		info.Candidates = candidates
	}
	return info
}

func taskTraceTeamTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func taskTraceTeamRandomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func taskTraceTeamNormalizeMembers(members []string, owner string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(members)+1)
	for _, member := range append([]string{owner}, members...) {
		member = taskTraceTeamMembershipName(member)
		if member == "" {
			continue
		}
		key := strings.ToLower(member)
		if !seen[key] {
			seen[key] = true
			result = append(result, member)
		}
	}
	return result
}

func taskTraceTeamEncodeLink(repository, shareID, secret string) string {
	return taskTraceTeamEncodeLinkPaths(repository, nil, shareID, secret)
}

func taskTraceTeamEncodeLinkPaths(repository string, repositories []string, shareID, secret string) string {
	paths := taskTraceTeamAppendRepository(nil, repository)
	for _, candidate := range repositories {
		paths = taskTraceTeamAppendRepository(paths, candidate)
	}
	primary := repository
	if len(paths) > 0 {
		primary = paths[0]
	}
	additional := []string{}
	if len(paths) > 1 {
		additional = paths[1:]
	}
	b, _ := json.Marshal(taskTraceTeamLink{Schema: taskTraceTeamSchema, Repository: primary, Repositories: additional, ShareID: shareID, Secret: secret})
	return "tasktrace-team://import/" + base64.RawURLEncoding.EncodeToString(b)
}

func taskTraceTeamDecodeLink(link string) (taskTraceTeamLink, error) {
	var parsed taskTraceTeamLink
	prefix := "tasktrace-team://import/"
	link = strings.TrimSpace(link)
	if !strings.HasPrefix(strings.ToLower(link), prefix) {
		return parsed, errors.New("invalid TaskTrace task link")
	}
	b, err := base64.RawURLEncoding.DecodeString(link[len(prefix):])
	if err != nil {
		return parsed, errors.New("invalid TaskTrace task link")
	}
	if err := json.Unmarshal(b, &parsed); err != nil || parsed.Schema != taskTraceTeamSchema || parsed.ShareID == "" || parsed.Repository == "" || parsed.Secret == "" {
		return parsed, errors.New("invalid TaskTrace task link")
	}
	return parsed, nil
}

func taskTraceTeamRepositoryCandidates(link taskTraceTeamLink, override string) []string {
	result := []string{}
	override = strings.TrimRight(strings.Trim(strings.TrimSpace(override), "\"'"), `\/`)
	if strings.HasPrefix(override, `\\`) {
		remainder := strings.TrimPrefix(override, `\\`)
		if !strings.ContainsAny(remainder, `\/`) {
			result = taskTraceTeamAppendRepository(result, override+`\teamData`)
		}
	}
	result = taskTraceTeamAppendRepository(result, override)
	result = taskTraceTeamAppendRepository(result, link.Repository)
	for _, repository := range link.Repositories {
		result = taskTraceTeamAppendRepository(result, repository)
	}
	return result
}

func taskTraceTeamReadLinkedManifest(link taskTraceTeamLink, override string) (string, TaskTraceTeamManifest, error) {
	var manifest TaskTraceTeamManifest
	candidates := taskTraceTeamRepositoryCandidates(link, override)
	var lastErr error
	for _, repository := range candidates {
		manifest = TaskTraceTeamManifest{}
		err := taskTraceTeamReadJSON(filepath.Join(taskTraceTeamShareDir(repository, link.ShareID), "manifest.json"), &manifest)
		if err == nil {
			return repository, manifest, nil
		}
		lastErr = err
	}
	if len(candidates) == 0 {
		return "", manifest, errors.New("team repository address is missing")
	}
	return "", manifest, fmt.Errorf("无法读取团队共享目录（已尝试 %s）。请确认共享目录地址形如 \\\\10.143.58.8\\teamData，并且当前 Windows 用户具有读写权限：%w", strings.Join(candidates, "、"), lastErr)
}

func taskTraceTeamShareDir(repository, shareID string) string {
	return filepath.Join(repository, "tasks", shareID)
}

func taskTraceTeamSnapshotPath(binding *TaskTraceTeamBinding, actor, device string) string {
	actor = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, actor)
	return filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "actors", actor, device+".json")
}

func taskTraceTeamFindBinding(state *taskTraceTeamState, shareID string) *TaskTraceTeamBinding {
	for i := range state.Bindings {
		if state.Bindings[i].ShareID == shareID {
			return &state.Bindings[i]
		}
	}
	return nil
}

func taskTraceTeamNodeForTask(binding *TaskTraceTeamBinding, taskID int64) string {
	for node, id := range binding.NodeTasks {
		if id == taskID {
			return node
		}
	}
	return ""
}

func taskTraceTeamSubtree(s *xorm.Session, root int64) ([]int64, map[int64]int64, error) {
	ids := []int64{}
	parents := map[int64]int64{}
	seen := map[int64]bool{}
	var walk func(int64) error
	walk = func(id int64) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		ids = append(ids, id)
		var relations []*TaskRelation
		if err := s.Where("task_id = ? AND relation_kind = ?", id, RelationKindSubtask).Find(&relations); err != nil {
			return err
		}
		sort.Slice(relations, func(i, j int) bool { return relations[i].OtherTaskID < relations[j].OtherTaskID })
		for _, relation := range relations {
			parents[relation.OtherTaskID] = id
			if err := walk(relation.OtherTaskID); err != nil {
				return err
			}
		}
		return nil
	}
	return ids, parents, walk(root)
}

type taskTraceTeamMarkerValue struct {
	ID     string `json:"id"`
	Author string `json:"author"`
}

func taskTraceTeamReadMarker(body string) (taskTraceTeamMarkerValue, bool) {
	match := taskTraceTeamMarker.FindStringSubmatch(body)
	if len(match) != 2 {
		return taskTraceTeamMarkerValue{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(match[1])
	if err != nil {
		return taskTraceTeamMarkerValue{}, false
	}
	var value taskTraceTeamMarkerValue
	if json.Unmarshal(b, &value) != nil || value.ID == "" {
		return taskTraceTeamMarkerValue{}, false
	}
	return value, true
}

func taskTraceTeamAddMarker(body, id, author string) string {
	if _, ok := taskTraceTeamReadMarker(body); ok {
		return body
	}
	b, _ := json.Marshal(taskTraceTeamMarkerValue{ID: id, Author: author})
	return body + "<!--tasktrace-team:" + base64.RawURLEncoding.EncodeToString(b) + "-->"
}

func taskTraceTeamIsOutstanding(body string) bool {
	return strings.Contains(body, "<h3>TaskTrace 遗留事项清单</h3>")
}

func taskTraceTeamOutstandingItems(body string) (map[string]string, []string) {
	items := map[string]string{}
	order := []string{}
	for _, match := range taskTraceTeamOutstandingItem.FindAllStringSubmatch(body, -1) {
		if len(match) != 3 || match[1] == "" {
			continue
		}
		if _, exists := items[match[1]]; !exists {
			order = append(order, match[1])
		}
		items[match[1]] = match[2]
	}
	return items, order
}

func taskTraceTeamOutstandingHTML(items map[string]string, order []string) string {
	seen := map[string]bool{}
	ids := make([]string, 0, len(items))
	for _, id := range order {
		if _, ok := items[id]; ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	extra := []string{}
	for id := range items {
		if !seen[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	ids = append(ids, extra...)
	var body strings.Builder
	body.WriteString("<h3>TaskTrace 遗留事项清单</h3><ul>")
	for _, id := range ids {
		body.WriteString(`<li data-id="`)
		body.WriteString(id)
		body.WriteString(`">`)
		body.WriteString(items[id])
		body.WriteString("</li>")
	}
	body.WriteString("</ul>")
	return body.String()
}

// Outstanding priorities are a personal display preference. They deliberately do
// not participate in the shared field value above, but must survive rebuilding a
// collaborative outstanding list on this device.
func taskTraceTeamOutstandingPriorities(body string) map[string]string {
	priorities := map[string]string{}
	for _, match := range taskTraceTeamOutstandingItem.FindAllStringSubmatchIndex(body, -1) {
		if len(match) < 6 {
			continue
		}
		id := body[match[2]:match[3]]
		opening := body[match[0]:match[4]]
		priority := taskTraceTeamOutstandingPriority.FindStringSubmatch(opening)
		if len(priority) == 2 {
			priorities[id] = priority[1]
		}
	}
	return priorities
}

func taskTraceTeamApplyOutstandingPriorities(body string, priorities map[string]string) string {
	if len(priorities) == 0 {
		return body
	}
	return taskTraceTeamOutstandingItem.ReplaceAllStringFunc(body, func(item string) string {
		match := taskTraceTeamOutstandingItem.FindStringSubmatch(item)
		if len(match) != 3 {
			return item
		}
		priority, ok := priorities[match[1]]
		if !ok {
			return item
		}
		openingEnd := strings.IndexByte(item, '>')
		if openingEnd < 0 {
			return item
		}
		opening := taskTraceTeamOutstandingPriority.ReplaceAllString(item[:openingEnd], "")
		return opening + ` data-priority="` + priority + `"` + item[openingEnd:]
	})
}

func taskTraceTeamCommentID(shareID, nodeID string, comment *TaskComment, actor string) string {
	if marker, ok := taskTraceTeamReadMarker(comment.Comment); ok {
		return marker.ID
	}
	source := shareID + "\x00" + nodeID + "\x00" + actor + "\x00" + strconv.FormatInt(comment.ID, 10) + "\x00" + comment.Created.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:16])
}

func taskTraceTeamAttachmentBlobPath(binding *TaskTraceTeamBinding, id string) string {
	return filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "attachments", id)
}

func taskTraceTeamLocalAttachmentKey(nodeID, attachmentID string) string {
	return nodeID + ":" + attachmentID
}

func taskTraceTeamExportAttachments(s *xorm.Session, binding *TaskTraceTeamBinding, taskID int64, nodeID string) ([]TaskTraceTeamAttachment, error) {
	attachments, err := getTaskAttachmentsByTaskIDs(s, []int64{taskID})
	if err != nil {
		return nil, err
	}
	if binding.LocalAttachments == nil {
		binding.LocalAttachments = map[string]int64{}
	}
	result := make([]TaskTraceTeamAttachment, 0, len(attachments))
	seen := map[string]bool{}
	for _, attachment := range attachments {
		if attachment.File == nil {
			continue
		}
		if err := attachment.File.LoadFileByID(); err != nil {
			return nil, err
		}
		content, readErr := io.ReadAll(attachment.File.File)
		_ = attachment.File.File.Close()
		if readErr != nil {
			return nil, readErr
		}
		sum := sha256.Sum256(content)
		id := hex.EncodeToString(sum[:])
		if seen[id] {
			continue
		}
		seen[id] = true
		binding.LocalAttachments[taskTraceTeamLocalAttachmentKey(nodeID, id)] = attachment.ID
		path := taskTraceTeamAttachmentBlobPath(binding, id)
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, err
			}
			tmp := path + ".tmp-" + uuid.NewString()
			if err := os.WriteFile(tmp, content, 0o600); err != nil {
				return nil, err
			}
			if err := os.Rename(tmp, path); err != nil {
				_ = os.Remove(tmp)
				if _, existingErr := os.Stat(path); existingErr != nil {
					return nil, err
				}
			}
		} else if statErr != nil {
			return nil, statErr
		}
		result = append(result, TaskTraceTeamAttachment{ID: id, Name: attachment.File.Name, Mime: attachment.File.Mime, Size: uint64(len(content)), SourceTaskID: taskID, SourceAttachmentID: attachment.ID})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func taskTraceTeamBuildSnapshot(s *xorm.Session, binding *TaskTraceTeamBinding, actor, device string) (TaskTraceTeamSnapshot, error) {
	ids, parents, err := taskTraceTeamSubtree(s, binding.RootTaskID)
	if err != nil {
		return TaskTraceTeamSnapshot{}, err
	}
	if binding.NodeTasks == nil {
		binding.NodeTasks = map[string]int64{}
	}
	for _, taskID := range ids {
		if taskTraceTeamNodeForTask(binding, taskID) == "" {
			binding.NodeTasks[uuid.NewString()] = taskID
		}
	}
	var manifest TaskTraceTeamManifest
	manifestAvailable := taskTraceTeamReadJSON(filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json"), &manifest) == nil
	acks := map[string]string{}
	for key, value := range binding.ResolutionAcks {
		acks[key] = value
	}
	snapshot := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: actor, DeviceID: device, Updated: time.Now().UTC(), Avatar: taskTraceTeamAvatarDataURI(s, actor), ResolutionAcks: acks, Tasks: []TaskTraceTeamTask{}}
	for _, taskID := range ids {
		task, err := GetTaskByIDSimple(s, taskID)
		if err != nil {
			return snapshot, err
		}
		node := taskTraceTeamNodeForTask(binding, taskID)
		parentNode := taskTraceTeamNodeForTask(binding, parents[taskID])
		comments, _, _, err := getAllCommentsForTasksWithoutPermissionCheck(s, []int64{taskID}, "", 1, -1, "asc")
		if err != nil {
			return snapshot, err
		}
		attachments, err := taskTraceTeamExportAttachments(s, binding, taskID, node)
		if err != nil {
			return snapshot, err
		}
		owner := actor
		if task.CreatedByID != 0 {
			if createdBy, ownerErr := user.GetUserByID(s, task.CreatedByID); ownerErr == nil && createdBy.Username != "" {
				owner = createdBy.Username
			}
		}
		assignees := []string{}
		if rows, assigneeErr := getRawTaskAssigneesForTasks(s, []int64{taskID}); assigneeErr == nil {
			for _, row := range rows {
				if row.Username != "" {
					assignees = append(assignees, row.Username)
				}
			}
			sort.Slice(assignees, func(i, j int) bool { return strings.ToLower(assignees[i]) < strings.ToLower(assignees[j]) })
		}
		if manifestAvailable {
			if permissions := manifest.Permissions[taskTraceTeamPermissionKey(node, "")]; len(permissions) > 0 {
				owner = taskTraceTeamPermissionOwner(permissions, owner)
				assignees = taskTraceTeamPermissionAssignees(permissions)
			}
		}
		shared := TaskTraceTeamTask{NodeID: node, ParentNode: parentNode, Title: task.Title, Description: task.Description, Owner: owner, Assignees: assignees, Done: task.Done, Status: task.Status, Updated: task.Updated, Comments: []TaskTraceTeamComment{}, Attachments: attachments}
		for _, comment := range comments {
			author := actor
			if marker, ok := taskTraceTeamReadMarker(comment.Comment); ok && marker.Author != "" {
				author = marker.Author
			} else if comment.Author != nil && comment.Author.Username != "" {
				author = comment.Author.Username
			}
			id := taskTraceTeamCommentID(binding.ShareID, node, comment, author)
			if taskTraceTeamIsOutstanding(comment.Comment) {
				shared.Outstanding = taskTraceTeamAddMarker(comment.Comment, id, author)
				continue
			}
			shared.Comments = append(shared.Comments, TaskTraceTeamComment{ID: id, Body: taskTraceTeamAddMarker(comment.Comment, id, author), Author: author, Created: comment.Created, Updated: comment.Updated})
		}
		snapshot.Tasks = append(snapshot.Tasks, shared)
	}
	return snapshot, nil
}

func taskTraceTeamAvatarDataURI(s *xorm.Session, username string) string {
	data, mime, err := avatar.GetAvatarForUsername(s, username, 64)
	if err != nil || len(data) == 0 || !taskTraceTeamAvatarMime[mime] {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

var taskTraceTeamAvatarMime = map[string]bool{
	"image/bmp":  true,
	"image/gif":  true,
	"image/jpeg": true,
	"image/png":  true,
	"image/tiff": true,
}

func taskTraceTeamSafeAvatar(value string) string {
	for mime := range taskTraceTeamAvatarMime {
		if strings.HasPrefix(value, "data:"+mime+";base64,") {
			return value
		}
	}
	return ""
}

func taskTraceTeamSnapshotHash(snapshot TaskTraceTeamSnapshot) string {
	normalized := snapshot
	normalized.Updated = time.Time{}
	normalized.Avatar = ""
	b, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type taskTraceTeamProgressChange struct {
	NodeID    string
	TaskTitle string
	Comment   TaskTraceTeamComment
}

func taskTraceTeamCommentContent(body string) string {
	return strings.TrimSpace(taskTraceTeamMarker.ReplaceAllString(body, ""))
}

func taskTraceTeamLatestProgressChange(previous *TaskTraceTeamSnapshot, current TaskTraceTeamSnapshot, actor string) *taskTraceTeamProgressChange {
	if previous == nil {
		return nil
	}
	previousComments := map[string]string{}
	for _, task := range previous.Tasks {
		for _, comment := range task.Comments {
			previousComments[task.NodeID+"\x00"+comment.ID] = taskTraceTeamCommentContent(comment.Body)
		}
	}
	var latest *taskTraceTeamProgressChange
	for _, task := range current.Tasks {
		for _, comment := range task.Comments {
			if !strings.EqualFold(comment.Author, actor) {
				continue
			}
			key := task.NodeID + "\x00" + comment.ID
			body := taskTraceTeamCommentContent(comment.Body)
			if previousBody, exists := previousComments[key]; exists && previousBody == body {
				continue
			}
			candidate := &taskTraceTeamProgressChange{NodeID: task.NodeID, TaskTitle: task.Title, Comment: comment}
			if latest == nil || candidate.Comment.Updated.After(latest.Comment.Updated) ||
				(candidate.Comment.Updated.Equal(latest.Comment.Updated) && candidate.Comment.Created.After(latest.Comment.Created)) {
				latest = candidate
			}
		}
	}
	return latest
}

func taskTraceTeamProgressNotificationID(shareID, actor string, change *taskTraceTeamProgressChange) string {
	when := change.Comment.Updated
	if when.IsZero() {
		when = change.Comment.Created
	}
	source := strings.Join([]string{
		shareID,
		strings.ToLower(actor),
		change.NodeID,
		change.Comment.ID,
		when.UTC().Format(time.RFC3339Nano),
		taskTraceTeamCommentContent(change.Comment.Body),
	}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(source)).String()
}

func taskTraceTeamBaseFromSnapshot(snapshot TaskTraceTeamSnapshot) map[string]TaskTraceTeamBase {
	base := map[string]TaskTraceTeamBase{}
	for _, task := range snapshot.Tasks {
		base[task.NodeID] = TaskTraceTeamBase{Title: task.Title, Description: task.Description, Done: task.Done, Status: task.Status, Outstanding: task.Outstanding}
	}
	return base
}

func taskTraceTeamWriteSnapshot(binding *TaskTraceTeamBinding, snapshot TaskTraceTeamSnapshot) error {
	return taskTraceTeamWriteJSON(taskTraceTeamSnapshotPath(binding, snapshot.Actor, snapshot.DeviceID), &snapshot)
}

func taskTraceTeamReadSnapshots(binding *TaskTraceTeamBinding) ([]TaskTraceTeamSnapshot, error) {
	root := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "actors")
	var result []TaskTraceTeamSnapshot
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		var snapshot TaskTraceTeamSnapshot
		if err := taskTraceTeamReadJSON(path, &snapshot); err != nil {
			return err
		}
		if snapshot.Schema == taskTraceTeamSchema && snapshot.ShareID == binding.ShareID {
			result = append(result, snapshot)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return result, nil
	}
	return result, err
}

func taskTraceTeamLatestActorSnapshots(snapshots []TaskTraceTeamSnapshot) []TaskTraceTeamSnapshot {
	grouped := map[string][]TaskTraceTeamSnapshot{}
	for _, snapshot := range snapshots {
		key := strings.ToLower(snapshot.Actor)
		grouped[key] = append(grouped[key], snapshot)
	}
	result := make([]TaskTraceTeamSnapshot, 0, len(grouped))
	for _, devices := range grouped {
		sort.Slice(devices, func(i, j int) bool { return devices[i].Updated.Before(devices[j].Updated) })
		combined := devices[len(devices)-1]
		tasks := map[string]TaskTraceTeamTask{}
		for _, device := range devices {
			for _, task := range device.Tasks {
				previous, exists := tasks[task.NodeID]
				// SQLite stores task timestamps with second precision. When two devices
				// changed within the same second, the later actor snapshot wins the
				// metadata tie while comments from both devices are still retained.
				if !exists || !task.Updated.Before(previous.Updated) {
					if exists {
						task.Comments = append(previous.Comments, task.Comments...)
						task.Attachments = append(previous.Attachments, task.Attachments...)
					}
					tasks[task.NodeID] = task
				} else {
					previous.Comments = append(previous.Comments, task.Comments...)
					previous.Attachments = append(previous.Attachments, task.Attachments...)
					tasks[task.NodeID] = previous
				}
			}
		}
		combined.Tasks = combined.Tasks[:0]
		for _, task := range tasks {
			comments := map[string]TaskTraceTeamComment{}
			for _, comment := range task.Comments {
				if previous, ok := comments[comment.ID]; !ok || comment.Updated.After(previous.Updated) {
					comments[comment.ID] = comment
				}
			}
			task.Comments = task.Comments[:0]
			for _, comment := range comments {
				task.Comments = append(task.Comments, comment)
			}
			sort.Slice(task.Comments, func(i, j int) bool { return task.Comments[i].Created.Before(task.Comments[j].Created) })
			attachments := map[string]TaskTraceTeamAttachment{}
			for _, attachment := range task.Attachments {
				attachments[attachment.ID] = attachment
			}
			task.Attachments = task.Attachments[:0]
			for _, attachment := range attachments {
				task.Attachments = append(task.Attachments, attachment)
			}
			sort.Slice(task.Attachments, func(i, j int) bool { return task.Attachments[i].ID < task.Attachments[j].ID })
			combined.Tasks = append(combined.Tasks, task)
		}
		sort.Slice(combined.Tasks, func(i, j int) bool { return combined.Tasks[i].NodeID < combined.Tasks[j].NodeID })
		result = append(result, combined)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Actor) < strings.ToLower(result[j].Actor) })
	return result
}

func taskTraceTeamTaskMap(snapshot TaskTraceTeamSnapshot) map[string]TaskTraceTeamTask {
	result := map[string]TaskTraceTeamTask{}
	for _, task := range snapshot.Tasks {
		result[task.NodeID] = task
	}
	return result
}

func taskTraceTeamFieldValue(task TaskTraceTeamTask, field string) string {
	if strings.HasPrefix(field, "outstanding:") {
		items, _ := taskTraceTeamOutstandingItems(task.Outstanding)
		return items[strings.TrimPrefix(field, "outstanding:")]
	}
	switch field {
	case "title":
		return task.Title
	case "done":
		return strconv.FormatBool(task.Done)
	case "status":
		return string(task.Status)
	}
	return ""
}

func taskTraceTeamBaseValue(base TaskTraceTeamBase, field string) string {
	if strings.HasPrefix(field, "outstanding:") {
		items, _ := taskTraceTeamOutstandingItems(base.Outstanding)
		return items[strings.TrimPrefix(field, "outstanding:")]
	}
	switch field {
	case "title":
		return base.Title
	case "done":
		return strconv.FormatBool(base.Done)
	case "status":
		return string(base.Status)
	}
	return ""
}

func taskTraceTeamConflictID(shareID, node, field string, options []TaskTraceTeamConflictOption) string {
	b, _ := json.Marshal(options)
	sum := sha256.Sum256(append([]byte(shareID+"\x00"+node+"\x00"+field+"\x00"), b...))
	return hex.EncodeToString(sum[:16])
}

func taskTraceTeamFindField(snapshots []TaskTraceTeamSnapshot, node, field string, base string, requiredResolution string) (string, []TaskTraceTeamConflictOption, bool) {
	values := map[string][]string{}
	key := node + ":" + field
	for _, snapshot := range snapshots {
		if requiredResolution != "" && snapshot.ResolutionAcks[key] != requiredResolution {
			continue
		}
		task, ok := taskTraceTeamTaskMap(snapshot)[node]
		if !ok {
			continue
		}
		value := taskTraceTeamFieldValue(task, field)
		if value != base {
			values[value] = append(values[value], snapshot.Actor)
		}
	}
	if len(values) == 0 {
		return base, nil, false
	}
	if len(values) == 1 {
		for value := range values {
			return value, nil, false
		}
	}
	options := make([]TaskTraceTeamConflictOption, 0, len(values))
	for value, actors := range values {
		sort.Strings(actors)
		options = append(options, TaskTraceTeamConflictOption{Author: strings.Join(actors, "、"), Value: value})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Author < options[j].Author })
	return base, options, true
}

func taskTraceTeamResolutionPath(binding *TaskTraceTeamBinding) string {
	return filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "resolutions.json")
}

func taskTraceTeamReadResolutions(binding *TaskTraceTeamBinding) (map[string]taskTraceTeamResolutionRecord, error) {
	result := map[string]taskTraceTeamResolutionRecord{}
	err := taskTraceTeamReadJSON(taskTraceTeamResolutionPath(binding), &result)
	if os.IsNotExist(err) {
		return result, nil
	}
	return result, err
}

func taskTraceTeamWriteResolutions(binding *TaskTraceTeamBinding, records map[string]taskTraceTeamResolutionRecord) error {
	return taskTraceTeamWriteJSON(taskTraceTeamResolutionPath(binding), &records)
}

func taskTraceTeamApplyResolution(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, record taskTraceTeamResolutionRecord) error {
	taskID := binding.NodeTasks[record.NodeID]
	if taskID == 0 {
		return nil
	}
	base := binding.Base[record.NodeID]
	switch record.Field {
	case "title":
		base.Title = record.Value
	case "done":
		base.Done, _ = strconv.ParseBool(record.Value)
	case "status":
		base.Status = TaskStatus(record.Value)
	default:
		if strings.HasPrefix(record.Field, "outstanding:") {
			items, order := taskTraceTeamOutstandingItems(base.Outstanding)
			id := strings.TrimPrefix(record.Field, "outstanding:")
			if record.Value == "" {
				delete(items, id)
			} else {
				items[id] = record.Value
			}
			base.Outstanding = taskTraceTeamOutstandingHTML(items, order)
		}
	}
	binding.Base[record.NodeID] = base
	if strings.HasPrefix(record.Field, "outstanding:") {
		return taskTraceTeamUpsertOutstanding(s, a, taskID, record.Value)
	}
	stored, err := GetTaskByIDSimple(s, taskID)
	if err != nil {
		return err
	}
	if base.Status == "" {
		if base.Done {
			base.Status = TaskStatusDone
		} else {
			base.Status = TaskStatusTodo
		}
	}
	return taskTraceTeamApplyTaskFields(s, a, taskID, base.Title, stored.Description, base.Done, base.Status)
}

func taskTraceTeamApplyPendingResolutions(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, records map[string]taskTraceTeamResolutionRecord) error {
	if binding.ResolutionAcks == nil {
		binding.ResolutionAcks = map[string]string{}
	}
	for key, record := range records {
		if binding.ResolutionAcks[key] == record.ID {
			continue
		}
		if err := taskTraceTeamApplyResolution(s, a, binding, record); err != nil {
			return err
		}
		binding.ResolutionAcks[key] = record.ID
	}
	return nil
}

func taskTraceTeamApplyTaskFields(s *xorm.Session, a web.Auth, taskID int64, title, description string, done bool, status TaskStatus) error {
	stored, err := GetTaskByIDSimple(s, taskID)
	if err != nil {
		return err
	}
	stored.Title = title
	stored.Description = description
	stored.Done = done
	stored.Status = status
	return stored.updateSingleTask(s, a, []string{"title", "description", "done", "status"})
}

func taskTraceTeamUpsertOutstanding(s *xorm.Session, a web.Auth, taskID int64, body string) error {
	var comments []*TaskComment
	if err := s.Where("task_id = ?", taskID).OrderBy("id desc").Find(&comments); err != nil {
		return err
	}
	for _, comment := range comments {
		if taskTraceTeamIsOutstanding(comment.Comment) {
			if comment.Comment == body {
				return nil
			}
			comment.Comment = body
			return comment.Update(s, a)
		}
	}
	if strings.TrimSpace(body) == "" {
		return nil
	}
	return (&TaskComment{TaskID: taskID, Comment: body}).Create(s, a)
}

func taskTraceTeamLocalCommentsByID(binding *TaskTraceTeamBinding, nodeID, actor string, comments []*TaskComment) (map[string]*TaskComment, []*TaskComment) {
	byID := map[string]*TaskComment{}
	duplicates := []*TaskComment{}
	for _, comment := range comments {
		id := ""
		if marker, ok := taskTraceTeamReadMarker(comment.Comment); ok {
			id = marker.ID
		} else {
			// A newly-created local comment has no team marker yet. Its exported id is
			// deterministic, so index it the same way before reading our own snapshot.
			// This lets the first sync attach the marker instead of creating a duplicate.
			id = taskTraceTeamCommentID(binding.ShareID, nodeID, comment, actor)
		}
		previous := byID[id]
		if previous == nil {
			byID[id] = comment
			continue
		}
		// Releases before the identity fix could create a marked copy beside the
		// original comment. Keep the newest copy (the marked one on a tie) and let
		// the next merge attach a marker if the original wins.
		_, commentMarked := taskTraceTeamReadMarker(comment.Comment)
		_, previousMarked := taskTraceTeamReadMarker(previous.Comment)
		if comment.Updated.After(previous.Updated) || (comment.Updated.Equal(previous.Updated) && commentMarked && !previousMarked) {
			duplicates = append(duplicates, previous)
			byID[id] = comment
		} else {
			duplicates = append(duplicates, comment)
		}
	}
	return byID, duplicates
}

func taskTraceTeamMergeComments(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, nodeID, actor string, taskID int64, remote []TaskTraceTeamComment) error {
	var local []*TaskComment
	if err := s.Where("task_id = ?", taskID).Find(&local); err != nil {
		return err
	}
	byID, duplicates := taskTraceTeamLocalCommentsByID(binding, nodeID, actor, local)
	for _, duplicate := range duplicates {
		if _, err := s.ID(duplicate.ID).NoAutoCondition().Delete(&TaskComment{}); err != nil {
			return err
		}
	}
	sort.Slice(remote, func(i, j int) bool { return remote[i].Created.Before(remote[j].Created) })
	for _, shared := range remote {
		body := taskTraceTeamAddMarker(shared.Body, shared.ID, shared.Author)
		if existing := byID[shared.ID]; existing != nil {
			_, marked := taskTraceTeamReadMarker(existing.Comment)
			if !marked || (existing.Comment != body && shared.Updated.After(existing.Updated)) {
				existing.Comment = body
				if err := existing.Update(s, a); err != nil {
					return err
				}
			}
			continue
		}
		comment := &TaskComment{TaskID: taskID, Comment: body, Created: shared.Created, Updated: shared.Updated}
		if comment.Created.IsZero() {
			comment.Created = time.Now().UTC()
		}
		if comment.Updated.IsZero() {
			comment.Updated = comment.Created
		}
		if err := comment.CreateWithTimestamps(s, a); err != nil {
			return err
		}
		byID[shared.ID] = comment
	}
	return nil
}

func taskTraceTeamCreateMissingTasks(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, snapshots []TaskTraceTeamSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	var richest TaskTraceTeamSnapshot
	for _, snapshot := range snapshots {
		if len(snapshot.Tasks) > len(richest.Tasks) {
			richest = snapshot
		}
	}
	if binding.NodeTasks == nil {
		binding.NodeTasks = map[string]int64{}
	}
	for pass := 0; pass < MaxTaskHierarchyDepth; pass++ {
		changed := false
		for _, shared := range richest.Tasks {
			if binding.NodeTasks[shared.NodeID] != 0 {
				continue
			}
			if shared.ParentNode != "" && binding.NodeTasks[shared.ParentNode] == 0 {
				continue
			}
			task := &Task{Title: shared.Title, Description: shared.Description, ProjectID: binding.ProjectID, Done: shared.Done, Status: shared.Status, Priority: 7}
			if err := task.Create(s, a); err != nil {
				return err
			}
			binding.NodeTasks[shared.NodeID] = task.ID
			if shared.ParentNode != "" {
				relation := &TaskRelation{TaskID: binding.NodeTasks[shared.ParentNode], OtherTaskID: task.ID, RelationKind: RelationKindSubtask}
				if err := relation.Create(s, a); err != nil {
					return err
				}
			}
			changed = true
		}
		if !changed {
			break
		}
	}
	return nil
}

func taskTraceTeamEnsureAttachments(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, snapshots []TaskTraceTeamSnapshot) error {
	if binding.LocalAttachments == nil {
		binding.LocalAttachments = map[string]int64{}
	}
	for _, snapshot := range snapshots {
		for _, task := range snapshot.Tasks {
			taskID := binding.NodeTasks[task.NodeID]
			if taskID == 0 {
				continue
			}
			for _, shared := range task.Attachments {
				key := taskTraceTeamLocalAttachmentKey(task.NodeID, shared.ID)
				if shared.ID == "" || binding.LocalAttachments[key] != 0 {
					continue
				}
				content, err := os.ReadFile(taskTraceTeamAttachmentBlobPath(binding, shared.ID))
				if err != nil {
					return fmt.Errorf("read shared attachment %s: %w", shared.Name, err)
				}
				sum := sha256.Sum256(content)
				if hex.EncodeToString(sum[:]) != shared.ID {
					return fmt.Errorf("shared attachment %s failed integrity check", shared.Name)
				}
				attachment := &TaskAttachment{TaskID: taskID}
				if err := attachment.NewAttachment(s, bytes.NewReader(content), shared.Name, uint64(len(content)), a); err != nil {
					return err
				}
				binding.LocalAttachments[key] = attachment.ID
			}
		}
	}
	return nil
}

func taskTraceTeamRewriteAttachments(body string, taskID int64, attachments []TaskTraceTeamAttachment, binding *TaskTraceTeamBinding) string {
	if body == "" {
		return body
	}
	nodeID := taskTraceTeamNodeForTask(binding, taskID)
	for _, attachment := range attachments {
		localID := binding.LocalAttachments[taskTraceTeamLocalAttachmentKey(nodeID, attachment.ID)]
		if localID == 0 {
			continue
		}
		for _, version := range []string{"v1", "v2"} {
			source := fmt.Sprintf("/api/%s/tasks/%d/attachments/%d", version, attachment.SourceTaskID, attachment.SourceAttachmentID)
			target := fmt.Sprintf("/api/%s/tasks/%d/attachments/%d", version, taskID, localID)
			body = strings.ReplaceAll(body, source, target)
		}
	}
	return body
}

func taskTraceTeamAllAttachments(rows []struct {
	actor string
	task  TaskTraceTeamTask
}) []TaskTraceTeamAttachment {
	byContent := map[string]TaskTraceTeamAttachment{}
	for _, row := range rows {
		for _, attachment := range row.task.Attachments {
			byContent[attachment.ID] = attachment
		}
	}
	result := make([]TaskTraceTeamAttachment, 0, len(byContent))
	for _, attachment := range byContent {
		result = append(result, attachment)
	}
	return result
}

func taskTraceTeamValidateBinding(binding *TaskTraceTeamBinding, actor string) error {
	var manifest TaskTraceTeamManifest
	path := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json")
	if err := taskTraceTeamReadJSON(path, &manifest); err != nil {
		return fmt.Errorf("open team repository: %w", err)
	}
	if manifest.ShareID != binding.ShareID || manifest.TokenHash != taskTraceTeamTokenHash(binding.Secret) {
		return errors.New("the task link is invalid or has been revoked")
	}
	for _, member := range manifest.Members {
		if strings.EqualFold(member, actor) {
			binding.Members = manifest.Members
			return nil
		}
	}
	return errors.New("you are no longer a member of this team task")
}

func taskTraceTeamMergeBinding(s *xorm.Session, a web.Auth, state *taskTraceTeamState, binding *TaskTraceTeamBinding, actor string) error {
	if err := taskTraceTeamValidateBinding(binding, actor); err != nil {
		return err
	}
	if root, err := GetTaskByIDSimple(s, binding.RootTaskID); err == nil {
		binding.ProjectID = root.ProjectID
	}
	resolutions, err := taskTraceTeamReadResolutions(binding)
	if err != nil {
		return err
	}
	if err := taskTraceTeamApplyPendingResolutions(s, a, binding, resolutions); err != nil {
		return err
	}
	local, err := taskTraceTeamBuildSnapshot(s, binding, actor, state.DeviceID)
	if err != nil {
		return err
	}
	var manifest TaskTraceTeamManifest
	manifestPath := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json")
	if err := taskTraceTeamReadJSON(manifestPath, &manifest); err != nil {
		return err
	}
	if changed, permissionErr := taskTraceTeamReconcileManifestPermissions(s, binding, &manifest, local, actor); permissionErr != nil {
		return permissionErr
	} else if changed {
		if err := taskTraceTeamWriteJSON(manifestPath, &manifest); err != nil {
			return err
		}
	}
	var previousLocal *TaskTraceTeamSnapshot
	var storedLocal TaskTraceTeamSnapshot
	if readErr := taskTraceTeamReadJSON(taskTraceTeamSnapshotPath(binding, actor, state.DeviceID), &storedLocal); readErr == nil {
		previousLocal = &storedLocal
	}
	taskTraceTeamProtectSnapshotPermissions(&local, previousLocal, &manifest, actor)
	progressChange := taskTraceTeamLatestProgressChange(previousLocal, local, actor)
	localHash := taskTraceTeamSnapshotHash(local)
	localChanged := binding.LastSnapshotHash != "" && binding.LastSnapshotHash != localHash
	if err := taskTraceTeamWriteSnapshot(binding, local); err != nil {
		return err
	}
	snapshots, err := taskTraceTeamReadSnapshots(binding)
	if err != nil {
		return err
	}
	snapshots = taskTraceTeamLatestActorSnapshots(snapshots)
	snapshots = taskTraceTeamFilterSnapshotsPermissions(snapshots, binding, &manifest)
	if err := taskTraceTeamCreateMissingTasks(s, a, binding, snapshots); err != nil {
		return err
	}
	if err := taskTraceTeamEnsureAttachments(s, a, binding, snapshots); err != nil {
		return err
	}
	allByNode := map[string][]struct {
		actor string
		task  TaskTraceTeamTask
	}{}
	for _, snapshot := range snapshots {
		for _, task := range snapshot.Tasks {
			allByNode[task.NodeID] = append(allByNode[task.NodeID], struct {
				actor string
				task  TaskTraceTeamTask
			}{snapshot.Actor, task})
		}
	}
	conflicts := []TaskTraceTeamConflict{}
	for node, taskID := range binding.NodeTasks {
		if !taskTraceTeamCan(&manifest, node, "", actor, false) {
			continue
		}
		rows := allByNode[node]
		if len(rows) == 0 {
			continue
		}
		base := binding.Base[node]
		stored, err := GetTaskByIDSimple(s, taskID)
		if err != nil {
			return err
		}
		title, done, status, outstanding := base.Title, base.Done, base.Status, base.Outstanding
		fields := []string{"title", "done", "status"}
		outstandingItems, outstandingOrder := taskTraceTeamOutstandingItems(base.Outstanding)
		outstandingIDs := map[string]bool{}
		for id := range outstandingItems {
			outstandingIDs[id] = true
		}
		for _, row := range rows {
			items, order := taskTraceTeamOutstandingItems(row.task.Outstanding)
			for _, id := range order {
				if !outstandingIDs[id] {
					outstandingOrder = append(outstandingOrder, id)
					outstandingIDs[id] = true
				}
			}
			for id := range items {
				outstandingIDs[id] = true
			}
		}
		for id := range outstandingIDs {
			fields = append(fields, "outstanding:"+id)
		}
		sort.Strings(fields[3:])
		for _, field := range fields {
			if strings.HasPrefix(field, "outstanding:") {
				outstandingID := strings.TrimPrefix(field, "outstanding:")
				if !taskTraceTeamCan(&manifest, node, outstandingID, actor, false) {
					delete(outstandingItems, outstandingID)
					continue
				}
			}
			key := node + ":" + field
			required := ""
			if resolution, ok := resolutions[key]; ok {
				required = resolution.ID
			}
			value, options, conflict := taskTraceTeamFindField(snapshots, node, field, taskTraceTeamBaseValue(base, field), required)
			if conflict {
				conflicts = append(conflicts, TaskTraceTeamConflict{ID: taskTraceTeamConflictID(binding.ShareID, node, field, options), ShareID: binding.ShareID, NodeID: node, TaskID: taskID, TaskTitle: stored.Title, Field: field, Base: taskTraceTeamBaseValue(base, field), Options: options})
				continue
			}
			switch field {
			case "title":
				title = value
			case "done":
				done, _ = strconv.ParseBool(value)
			case "status":
				status = TaskStatus(value)
			default:
				if strings.HasPrefix(field, "outstanding:") {
					id := strings.TrimPrefix(field, "outstanding:")
					if value == "" {
						delete(outstandingItems, id)
					} else {
						outstandingItems[id] = value
					}
				}
			}
		}
		outstanding = taskTraceTeamOutstandingHTML(outstandingItems, outstandingOrder)
		if localTask, ok := taskTraceTeamTaskMap(local)[node]; ok {
			outstanding = taskTraceTeamApplyOutstandingPriorities(outstanding, taskTraceTeamOutstandingPriorities(localTask.Outstanding))
		}
		latest := rows[0].task
		for _, row := range rows[1:] {
			if row.task.Updated.After(latest.Updated) {
				latest = row.task
			}
		}
		attachments := taskTraceTeamAllAttachments(rows)
		description := taskTraceTeamRewriteAttachments(latest.Description, taskID, attachments, binding)
		outstanding = taskTraceTeamRewriteAttachments(outstanding, taskID, attachments, binding)
		if status == "" {
			if done {
				status = TaskStatusDone
			} else {
				status = TaskStatusTodo
			}
		}
		if stored.Title != title || stored.Description != description || stored.Done != done || stored.Status != status {
			if err := taskTraceTeamApplyTaskFields(s, a, taskID, title, description, done, status); err != nil {
				return err
			}
		}
		if taskTraceTeamBaseValue(base, "outstanding") != outstanding || outstanding != "" {
			if err := taskTraceTeamUpsertOutstanding(s, a, taskID, outstanding); err != nil {
				return err
			}
		}
		commentByID := map[string]TaskTraceTeamComment{}
		for _, row := range rows {
			for _, comment := range row.task.Comments {
				comment.Body = taskTraceTeamRewriteAttachments(comment.Body, taskID, row.task.Attachments, binding)
				if previous, ok := commentByID[comment.ID]; !ok || comment.Updated.After(previous.Updated) {
					commentByID[comment.ID] = comment
				}
			}
		}
		comments := make([]TaskTraceTeamComment, 0, len(commentByID))
		for _, comment := range commentByID {
			comments = append(comments, comment)
		}
		if err := taskTraceTeamMergeComments(s, a, binding, node, actor, taskID, comments); err != nil {
			return err
		}
		binding.Base[node] = TaskTraceTeamBase{Title: title, Description: description, Done: done, Status: status, Outstanding: outstanding}
	}
	binding.Conflicts = conflicts
	binding.LastSync = time.Now().UTC()
	binding.LastError = ""
	if binding.Notify && localChanged && progressChange != nil {
		created := progressChange.Comment.Updated
		if created.IsZero() {
			created = progressChange.Comment.Created
		}
		if created.IsZero() {
			created = time.Now().UTC()
		}
		notice := TaskTraceTeamNotification{
			ID:              taskTraceTeamProgressNotificationID(binding.ShareID, actor, progressChange),
			ShareID:         binding.ShareID,
			Actor:           actor,
			Avatar:          local.Avatar,
			TaskTitle:       progressChange.TaskTitle,
			NodeID:          progressChange.NodeID,
			SharedCommentID: progressChange.Comment.ID,
			Created:         created.UTC(),
		}
		for _, member := range binding.Members {
			if strings.EqualFold(member, actor) {
				continue
			}
			path := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "notifications", strings.ToLower(member), notice.ID+".json")
			_ = taskTraceTeamWriteJSON(path, &notice)
		}
	}
	// Rebuild after merging. The next sync must compare against the state the user
	// now sees, otherwise a collaborator's imported change is reported as our own.
	final, err := taskTraceTeamBuildSnapshot(s, binding, actor, state.DeviceID)
	if err != nil {
		return err
	}
	if err := taskTraceTeamWriteSnapshot(binding, final); err != nil {
		return err
	}
	binding.LastSnapshotHash = taskTraceTeamSnapshotHash(final)
	return nil
}

func taskTraceTeamUpdateMembersLocked(s *xorm.Session, a web.Auth, state taskTraceTeamState, binding *TaskTraceTeamBinding, requested []string, actor string) (*TaskTraceTeamStatus, error) {
	resolvedMembers := make([]string, 0, len(requested))
	for _, member := range requested {
		if taskTraceTeamMembersEqual(member, actor) {
			continue
		}
		resolved := ""
		for _, existing := range binding.Members {
			if taskTraceTeamMembersEqual(existing, member) {
				resolved = existing
				break
			}
		}
		if resolved == "" {
			var err error
			resolved, err = taskTraceTeamGrantWindowsAccess(binding.Repository, member)
			if err != nil {
				return nil, fmt.Errorf("无法为 %s 设置 teamData 读写权限：%w", member, err)
			}
		}
		resolvedMembers = append(resolvedMembers, resolved)
	}
	members := taskTraceTeamNormalizeMembers(resolvedMembers, actor)
	if len(members) < 2 {
		return nil, errors.New("at least one other team member is required")
	}
	manifestPath := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json")
	var manifest TaskTraceTeamManifest
	if err := taskTraceTeamReadJSON(manifestPath, &manifest); err != nil {
		return nil, err
	}
	for _, permissions := range manifest.Permissions {
		for _, permission := range permissions {
			if permission.Assignee && !taskTraceTeamContainsMember(members, permission.Username) {
				return nil, fmt.Errorf("请先取消 %s 的受理人身份，再从协作成员中移除", permission.Username)
			}
		}
	}
	manifest.Members = members
	for key, permissions := range manifest.Permissions {
		manifest.Permissions[key] = taskTraceTeamNormalizePermissions(members, taskTraceTeamPermissionOwner(permissions, manifest.Owner), taskTraceTeamPermissionAssignees(permissions), permissions)
	}
	manifest.Updated = time.Now().UTC()
	if err := taskTraceTeamWriteJSON(manifestPath, &manifest); err != nil {
		return nil, err
	}
	binding.Members = members
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLocked(s, a, state)
	return &status, err
}

func TaskTraceTeamShare(s *xorm.Session, a web.Auth, request TaskTraceTeamShareRequest) (*TaskTraceTeamStatus, error) {
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	if !taskTraceTeamEnabled() {
		return nil, errors.New("TaskTrace team sync is unavailable")
	}
	u, err := user.GetFromAuth(a)
	if err != nil {
		return nil, err
	}
	can, err := (&Task{ID: request.TaskID}).CanWrite(s, a)
	if err != nil || !can {
		if err != nil {
			return nil, err
		}
		return nil, ErrGenericForbidden{}
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	for index := range state.Bindings {
		existing := &state.Bindings[index]
		if existing.RootTaskID != request.TaskID {
			continue
		}
		if !strings.EqualFold(existing.Owner, u.Username) {
			return nil, ErrGenericForbidden{}
		}
		return taskTraceTeamUpdateMembersLocked(s, a, state, existing, request.Members, u.Username)
	}
	root := taskTraceTeamRoot()
	grantedMembers := make([]string, 0, len(request.Members))
	for _, member := range request.Members {
		if taskTraceTeamMembersEqual(member, u.Username) {
			continue
		}
		resolved, grantErr := taskTraceTeamGrantWindowsAccess(root, member)
		if grantErr != nil {
			return nil, fmt.Errorf("无法为 %s 设置 teamData 读写权限：%w", member, grantErr)
		}
		grantedMembers = append(grantedMembers, resolved)
	}
	members := taskTraceTeamNormalizeMembers(grantedMembers, u.Username)
	if len(members) < 2 {
		return nil, errors.New("at least one other team member is required")
	}
	secret, err := taskTraceTeamRandomSecret()
	if err != nil {
		return nil, err
	}
	shareID := uuid.NewString()
	info := taskTraceTeamRepositoryInfo(root)
	binding := TaskTraceTeamBinding{ShareID: shareID, Repository: root, Secret: secret, Owner: u.Username, Members: members, RootTaskID: request.TaskID, NodeTasks: map[string]int64{}, Base: map[string]TaskTraceTeamBase{}, ResolutionAcks: map[string]string{}, LocalAttachments: map[string]int64{}, Notify: true}
	snapshot, err := taskTraceTeamBuildSnapshot(s, &binding, u.Username, state.DeviceID)
	if err != nil {
		return nil, err
	}
	if len(snapshot.Tasks) == 0 {
		return nil, errors.New("task subtree is empty")
	}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: shareID, RootNode: snapshot.Tasks[0].NodeID, Owner: u.Username, Members: members, Permissions: map[string][]TaskTraceTeamMemberPermission{}, TokenHash: taskTraceTeamTokenHash(secret), Created: time.Now().UTC(), Updated: time.Now().UTC()}
	if _, err := taskTraceTeamReconcileManifestPermissions(s, &binding, &manifest, snapshot, u.Username); err != nil {
		return nil, err
	}
	if err := taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(root, shareID), "manifest.json"), &manifest); err != nil {
		return nil, err
	}
	if err := taskTraceTeamWriteSnapshot(&binding, snapshot); err != nil {
		return nil, err
	}
	binding.Base = taskTraceTeamBaseFromSnapshot(snapshot)
	binding.LastSnapshotHash = taskTraceTeamSnapshotHash(snapshot)
	binding.LastSync = time.Now().UTC()
	if task, err := GetTaskByIDSimple(s, request.TaskID); err == nil {
		binding.ProjectID = task.ProjectID
	}
	state.Bindings = append(state.Bindings, binding)
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	_ = info
	status, err := taskTraceTeamStatusLocked(s, a, state)
	return &status, err
}

func TaskTraceTeamImport(s *xorm.Session, a web.Auth, request TaskTraceTeamImportRequest) (*TaskTraceTeamStatus, error) {
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	if !taskTraceTeamEnabled() {
		return nil, errors.New("TaskTrace team sync is unavailable")
	}
	if request.ProjectID <= 0 {
		return nil, errors.New("a target project is required")
	}
	u, err := user.GetFromAuth(a)
	if err != nil {
		return nil, err
	}
	can, err := (&Project{ID: request.ProjectID}).CanWrite(s, a)
	if err != nil || !can {
		if err != nil {
			return nil, err
		}
		return nil, ErrGenericForbidden{}
	}
	link, err := taskTraceTeamDecodeLink(request.Link)
	if err != nil {
		return nil, err
	}
	repository, manifest, err := taskTraceTeamReadLinkedManifest(link, "")
	if err != nil {
		return nil, err
	}
	if manifest.ShareID != link.ShareID || manifest.TokenHash != taskTraceTeamTokenHash(link.Secret) {
		return nil, errors.New("the task link is invalid or has been revoked")
	}
	isMember := false
	for _, member := range manifest.Members {
		if taskTraceTeamMembersEqual(member, u.Username) {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, fmt.Errorf("当前 Windows 用户 %q 不在任务共享成员列表中（%s）", u.Username, strings.Join(manifest.Members, "、"))
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	if taskTraceTeamFindBinding(&state, link.ShareID) != nil {
		return nil, errors.New("this team task has already been imported")
	}
	binding := TaskTraceTeamBinding{ShareID: link.ShareID, Repository: repository, Secret: link.Secret, Owner: manifest.Owner, Members: manifest.Members, ProjectID: request.ProjectID, NodeTasks: map[string]int64{}, Base: map[string]TaskTraceTeamBase{}, ResolutionAcks: map[string]string{}, LocalAttachments: map[string]int64{}, Notify: true}
	snapshots, err := taskTraceTeamReadSnapshots(&binding)
	if err != nil {
		return nil, fmt.Errorf("read team task: %w", err)
	}
	snapshots = taskTraceTeamLatestActorSnapshots(snapshots)
	if len(snapshots) == 0 {
		return nil, errors.New("the shared task has no data")
	}
	if err := taskTraceTeamCreateMissingTasks(s, a, &binding, snapshots); err != nil {
		return nil, err
	}
	binding.RootTaskID = binding.NodeTasks[manifest.RootNode]
	if binding.RootTaskID == 0 {
		return nil, errors.New("the shared task root is missing")
	}
	owner := snapshots[0]
	for _, snapshot := range snapshots {
		if strings.EqualFold(snapshot.Actor, manifest.Owner) {
			owner = snapshot
			break
		}
	}
	binding.Base = taskTraceTeamBaseFromSnapshot(owner)
	manifest.Members = binding.Members
	manifest.Updated = time.Now().UTC()
	if err := taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, link.ShareID), "manifest.json"), &manifest); err != nil {
		return nil, err
	}
	state.Bindings = append(state.Bindings, binding)
	if err := taskTraceTeamMergeBinding(s, a, &state, &state.Bindings[len(state.Bindings)-1], u.Username); err != nil {
		return nil, err
	}
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLocked(s, a, state)
	return &status, err
}

func TaskTraceTeamSync(s *xorm.Session, a web.Auth) (*TaskTraceTeamStatus, error) {
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	if !taskTraceTeamEnabled() {
		return nil, errors.New("TaskTrace team sync is unavailable")
	}
	u, err := user.GetFromAuth(a)
	if err != nil {
		return nil, err
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	for i := range state.Bindings {
		binding := &state.Bindings[i]
		if err := taskTraceTeamMergeBinding(s, a, &state, binding, u.Username); err != nil {
			binding.LastError = err.Error()
		}
	}
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLocked(s, a, state)
	return &status, err
}

func TaskTraceTeamConfigure(s *xorm.Session, a web.Auth, request TaskTraceTeamConfigureRequest) (*TaskTraceTeamStatus, error) {
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	binding := taskTraceTeamFindBinding(&state, request.ShareID)
	if binding == nil {
		return nil, errors.New("team task not found")
	}
	can, err := (&Task{ID: binding.RootTaskID}).CanWrite(s, a)
	if err != nil || !can {
		if err != nil {
			return nil, err
		}
		return nil, ErrGenericForbidden{}
	}
	binding.Notify = request.Notify
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLocked(s, a, state)
	return &status, err
}

func TaskTraceTeamResolve(s *xorm.Session, a web.Auth, request TaskTraceTeamResolveRequest) (*TaskTraceTeamStatus, error) {
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	binding := taskTraceTeamFindBinding(&state, request.ShareID)
	if binding == nil {
		return nil, errors.New("team task not found")
	}
	records, err := taskTraceTeamReadResolutions(binding)
	if err != nil {
		return nil, err
	}
	if binding.ResolutionAcks == nil {
		binding.ResolutionAcks = map[string]string{}
	}
	values := map[string]string{}
	for _, resolution := range request.Resolutions {
		values[resolution.ConflictID] = resolution.Value
	}
	remaining := []TaskTraceTeamConflict{}
	for _, conflict := range binding.Conflicts {
		value, ok := values[conflict.ID]
		if !ok {
			remaining = append(remaining, conflict)
			continue
		}
		valid := value == conflict.Base
		for _, option := range conflict.Options {
			if option.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			return nil, errors.New("invalid conflict resolution")
		}
		key := conflict.NodeID + ":" + conflict.Field
		record := taskTraceTeamResolutionRecord{ID: uuid.NewString(), NodeID: conflict.NodeID, Field: conflict.Field, Value: value, Updated: time.Now().UTC()}
		records[key] = record
		if err := taskTraceTeamApplyResolution(s, a, binding, record); err != nil {
			return nil, err
		}
		binding.ResolutionAcks[key] = record.ID
	}
	binding.Conflicts = remaining
	if err := taskTraceTeamWriteResolutions(binding, records); err != nil {
		return nil, err
	}
	u, _ := user.GetFromAuth(a)
	snapshot, err := taskTraceTeamBuildSnapshot(s, binding, u.Username, state.DeviceID)
	if err != nil {
		return nil, err
	}
	if err := taskTraceTeamWriteSnapshot(binding, snapshot); err != nil {
		return nil, err
	}
	binding.LastSnapshotHash = taskTraceTeamSnapshotHash(snapshot)
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLocked(s, a, state)
	return &status, err
}

func TaskTraceTeamNotificationsRead(s *xorm.Session, a web.Auth, request TaskTraceTeamNotificationsReadRequest) (*TaskTraceTeamStatus, error) {
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	u, err := user.GetFromAuth(a)
	if err != nil {
		return nil, err
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, id := range request.IDs {
		if parsed, parseErr := uuid.Parse(id); parseErr == nil {
			selected[parsed.String()] = true
		}
	}
	for i := range state.Bindings {
		dir := filepath.Join(taskTraceTeamShareDir(state.Bindings[i].Repository, state.Bindings[i].ShareID), "notifications", strings.ToLower(u.Username))
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			continue
		}
		for _, entry := range entries {
			id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if entry.IsDir() || (!selected[id] && len(selected) > 0) {
				continue
			}
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	status, err := taskTraceTeamStatusLocked(s, a, state)
	return &status, err
}

func taskTraceTeamResolveNotificationTarget(s *xorm.Session, binding *TaskTraceTeamBinding, notice *TaskTraceTeamNotification) {
	if notice.NodeID == "" {
		return
	}
	notice.TaskID = binding.NodeTasks[notice.NodeID]
	if notice.TaskID == 0 || notice.SharedCommentID == "" {
		return
	}
	var comments []*TaskComment
	if err := s.Where("task_id = ?", notice.TaskID).Find(&comments); err != nil {
		return
	}
	for _, comment := range comments {
		if marker, ok := taskTraceTeamReadMarker(comment.Comment); ok && marker.ID == notice.SharedCommentID {
			notice.CommentID = comment.ID
			return
		}
	}
}

func taskTraceTeamNotifications(s *xorm.Session, binding *TaskTraceTeamBinding, username string) []TaskTraceTeamNotification {
	dir := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "notifications", strings.ToLower(username))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	result := []TaskTraceTeamNotification{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		var notice TaskTraceTeamNotification
		if taskTraceTeamReadJSON(filepath.Join(dir, entry.Name()), &notice) == nil {
			notice.Avatar = taskTraceTeamSafeAvatar(notice.Avatar)
			taskTraceTeamResolveNotificationTarget(s, binding, &notice)
			result = append(result, notice)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Created.After(result[j].Created) })
	return result
}

func taskTraceTeamProfiles(binding *TaskTraceTeamBinding) []TaskTraceTeamMemberProfile {
	profiles := map[string]TaskTraceTeamMemberProfile{}
	for _, member := range binding.Members {
		profiles[strings.ToLower(member)] = TaskTraceTeamMemberProfile{Username: member}
	}
	snapshots, err := taskTraceTeamReadSnapshots(binding)
	if err == nil {
		for _, snapshot := range taskTraceTeamLatestActorSnapshots(snapshots) {
			key := strings.ToLower(snapshot.Actor)
			profile := profiles[key]
			profile.Username = snapshot.Actor
			if safe := taskTraceTeamSafeAvatar(snapshot.Avatar); safe != "" {
				profile.Avatar = safe
			}
			profiles[key] = profile
		}
	}
	result := make([]TaskTraceTeamMemberProfile, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, profile)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Username) < strings.ToLower(result[j].Username) })
	return result
}

func taskTraceTeamStatusLocked(s *xorm.Session, a web.Auth, state taskTraceTeamState) (TaskTraceTeamStatus, error) {
	u, err := user.GetFromAuth(a)
	if err != nil {
		return TaskTraceTeamStatus{}, err
	}
	root := taskTraceTeamRoot()
	status := TaskTraceTeamStatus{Enabled: taskTraceTeamEnabled(), Username: u.Username, Repository: taskTraceTeamRepositoryInfo(root), UnassignedMembers: []string{}, Bindings: []TaskTraceTeamBindingStatus{}, Conflicts: []TaskTraceTeamConflict{}, Notifications: []TaskTraceTeamNotification{}, Profiles: []TaskTraceTeamMemberProfile{}}
	status.UnassignedMembers = taskTraceTeamUnassignedMembers(state, status.Repository.Candidates, u.Username)
	profiles := map[string]TaskTraceTeamMemberProfile{
		strings.ToLower(u.Username): {Username: u.Username, Avatar: taskTraceTeamAvatarDataURI(s, u.Username)},
	}
	for _, binding := range state.Bindings {
		manifest := TaskTraceTeamManifest{Owner: binding.Owner, Members: binding.Members}
		if manifestErr := taskTraceTeamReadJSON(filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json"), &manifest); manifestErr != nil {
			manifest = TaskTraceTeamManifest{Owner: binding.Owner, Members: binding.Members}
		}
		ids := make([]int64, 0, len(binding.NodeTasks))
		for _, id := range binding.NodeTasks {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		linkPath := binding.Repository
		linkPaths := []string{}
		if binding.Owner == u.Username {
			info := taskTraceTeamRepositoryInfo(binding.Repository)
			linkPath = info.Path
			linkPaths = info.Paths
		}
		rootTitle := ""
		if task, taskErr := GetTaskByIDSimple(s, binding.RootTaskID); taskErr == nil {
			rootTitle = task.Title
		}
		permissionTargets := taskTraceTeamPermissionTargets(s, &binding, &manifest, u.Username)
		canManagePermissions := strings.EqualFold(binding.Owner, u.Username)
		if !canManagePermissions {
			for _, target := range permissionTargets {
				if target.CanManage {
					canManagePermissions = true
					break
				}
			}
		}
		row := TaskTraceTeamBindingStatus{
			ShareID: binding.ShareID, Owner: binding.Owner, Members: manifest.Members, RootTaskID: binding.RootTaskID, RootTaskTitle: rootTitle,
			TaskIDs: ids, Link: taskTraceTeamEncodeLinkPaths(linkPath, linkPaths, binding.ShareID, binding.Secret), MemberLink: taskTraceTeamEncodeMembersLink(manifest.Members),
			Notify: binding.Notify, CanManagePermissions: canManagePermissions,
			PermissionTargets: permissionTargets,
			LastSync:          binding.LastSync, LastError: binding.LastError, Conflicts: binding.Conflicts,
		}
		status.Bindings = append(status.Bindings, row)
		status.Conflicts = append(status.Conflicts, binding.Conflicts...)
		status.Notifications = append(status.Notifications, taskTraceTeamNotifications(s, &binding, u.Username)...)
		for _, profile := range taskTraceTeamProfiles(&binding) {
			key := strings.ToLower(profile.Username)
			if current := profiles[key]; current.Avatar != "" && profile.Avatar == "" {
				continue
			}
			profiles[key] = profile
		}
	}
	for _, profile := range profiles {
		status.Profiles = append(status.Profiles, profile)
	}
	sort.Slice(status.Profiles, func(i, j int) bool {
		return strings.ToLower(status.Profiles[i].Username) < strings.ToLower(status.Profiles[j].Username)
	})
	avatarByActor := map[string]string{}
	for _, profile := range status.Profiles {
		avatarByActor[strings.ToLower(profile.Username)] = profile.Avatar
	}
	for i := range status.Notifications {
		if status.Notifications[i].Avatar == "" {
			status.Notifications[i].Avatar = avatarByActor[strings.ToLower(status.Notifications[i].Actor)]
		}
	}
	sort.Slice(status.Notifications, func(i, j int) bool { return status.Notifications[i].Created.After(status.Notifications[j].Created) })
	return status, nil
}

func TaskTraceTeamReadStatus(s *xorm.Session, a web.Auth) (TaskTraceTeamStatus, error) {
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	if !taskTraceTeamEnabled() {
		return TaskTraceTeamStatus{Enabled: false}, nil
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return TaskTraceTeamStatus{}, err
	}
	return taskTraceTeamStatusLocked(s, a, state)
}
