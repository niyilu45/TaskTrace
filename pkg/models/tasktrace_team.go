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
	"unicode"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/modules/avatar"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"github.com/google/uuid"
	"golang.org/x/net/html"
	"xorm.io/xorm"
)

const taskTraceTeamSchema = 1

const (
	taskTraceTeamLockHeartbeat  = 5 * time.Second
	taskTraceTeamLockStaleAfter = 30 * time.Second
)

var taskTraceTeamMu sync.Mutex

type TaskTraceTeamRepositoryInfo struct {
	Path       string                          `json:"path"`
	Paths      []string                        `json:"-"`
	Computer   string                          `json:"computer,omitempty"`
	Candidates []string                        `json:"candidates"`
	Members    []TaskTraceTeamRepositoryMember `json:"members" readOnly:"true" doc:"Windows accounts with access to teamData and their maximum access level."`
	Shared     bool                            `json:"shared"`
}

const (
	TaskTraceTeamAccessRead  = "read"
	TaskTraceTeamAccessWrite = "write"
)

type TaskTraceTeamRepositoryMember struct {
	AccountName string `json:"account_name" readOnly:"true"`
	Access      string `json:"access" readOnly:"true" enum:"read,write"`
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
	Deleted bool      `json:"deleted,omitempty"`
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
	Schema         int                          `json:"schema"`
	ShareID        string                       `json:"share_id"`
	Actor          string                       `json:"actor"`
	DeviceID       string                       `json:"device_id"`
	Updated        time.Time                    `json:"updated"`
	Avatar         string                       `json:"avatar,omitempty"`
	ResolutionAcks map[string]string            `json:"resolution_acks,omitempty"`
	Base           map[string]TaskTraceTeamBase `json:"base,omitempty"`
	Tasks          []TaskTraceTeamTask          `json:"tasks"`
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
	ShareID               string                       `json:"share_id"`
	Repository            string                       `json:"repository"`
	Secret                string                       `json:"secret"`
	Owner                 string                       `json:"owner"`
	Members               []string                     `json:"members"`
	RootTaskID            int64                        `json:"root_task_id"`
	ProjectID             int64                        `json:"project_id"`
	NodeTasks             map[string]int64             `json:"node_tasks"`
	Base                  map[string]TaskTraceTeamBase `json:"base"`
	Notify                bool                         `json:"notify"`
	LastSnapshotHash      string                       `json:"last_snapshot_hash,omitempty"`
	LastSync              time.Time                    `json:"last_sync,omitempty"`
	LastError             string                       `json:"last_error,omitempty"`
	Conflicts             []TaskTraceTeamConflict      `json:"conflicts,omitempty"`
	ResolutionAcks        map[string]string            `json:"resolution_acks,omitempty"`
	LocalAttachments      map[string]int64             `json:"local_attachments,omitempty"`
	OutstandingPriorities map[string]int               `json:"outstanding_priorities,omitempty"`
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
	Username    string `json:"username"`
	AccountName string `json:"account_name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

type TaskTraceTeamStatus struct {
	Enabled           bool                         `json:"enabled"`
	Username          string                       `json:"username"`
	Repository        TaskTraceTeamRepositoryInfo  `json:"repository"`
	UnassignedMembers []string                     `json:"unassigned_members" readOnly:"true" doc:"Accounts with teamData access that do not belong to any collaboration team."`
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
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = os.Remove(tmp)
		}
	}()
	if written, writeErr := file.Write(b); writeErr != nil {
		return writeErr
	} else if written != len(b) {
		return io.ErrShortWrite
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func taskTraceTeamAcquireShareLock(binding *TaskTraceTeamBinding) (func(), error) {
	path := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), ".tasktrace-write.lock")
	deadline := time.Now().Add(10 * time.Second)
	for {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			token := uuid.NewString()
			_, _ = fmt.Fprintf(file, "%s\n%d\n", token, time.Now().UTC().UnixNano())
			_ = file.Sync()
			stop := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(taskTraceTeamLockHeartbeat)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						content, readErr := os.ReadFile(path)
						if readErr != nil || !strings.HasPrefix(string(content), token+"\n") {
							return
						}
						now := time.Now()
						_ = os.Chtimes(path, now, now)
					case <-stop:
						return
					}
				}
			}()
			var once sync.Once
			return func() {
				once.Do(func() {
					close(stop)
					<-done
					_ = file.Close()
					content, readErr := os.ReadFile(path)
					if readErr == nil && strings.HasPrefix(string(content), token+"\n") {
						_ = os.Remove(path)
					}
				})
			}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > taskTraceTeamLockStaleAfter {
			if removeErr := os.Remove(path); removeErr == nil || os.IsNotExist(removeErr) {
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, errors.New("another computer is updating this collaborative task; retry shortly")
		}
		time.Sleep(50 * time.Millisecond)
	}
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

func taskTraceTeamRepositoryMetadata(root string) TaskTraceTeamRepositoryInfo {
	info := TaskTraceTeamRepositoryInfo{Path: root, Paths: []string{}, Candidates: []string{}, Members: []TaskTraceTeamRepositoryMember{}}
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
	return info
}

func taskTraceTeamRepositoryInfo(root string) TaskTraceTeamRepositoryInfo {
	info := taskTraceTeamRepositoryMetadata(root)
	if members, err := taskTraceTeamListWindowsAccess(root); err == nil {
		info.Members = taskTraceTeamFilterRepositoryMembers(members)
		info.Candidates = taskTraceTeamRepositoryMemberNames(info.Members)
	}
	return info
}

func taskTraceTeamRepositoryInfoCached(root string) TaskTraceTeamRepositoryInfo {
	info := taskTraceTeamRepositoryMetadata(root)
	if members, ok := taskTraceWindowsAccessCache.cached(root); ok {
		info.Members = taskTraceTeamFilterRepositoryMembers(members)
		info.Candidates = taskTraceTeamRepositoryMemberNames(info.Members)
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
		isOwner := owner != "" && taskTraceTeamMembersEqual(member, owner)
		if !isOwner && len(taskTraceTeamFilterAccessMembers([]string{member})) == 0 {
			continue
		}
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
	visiting := map[int64]bool{}
	var walk func(int64, int) error
	walk = func(id int64, depth int) error {
		if visiting[id] {
			return fmt.Errorf("task hierarchy contains a cycle at task %d", id)
		}
		if seen[id] {
			return nil
		}
		if depth > MaxTaskHierarchyDepth {
			return fmt.Errorf("task hierarchy exceeds %d levels at task %d", MaxTaskHierarchyDepth, id)
		}
		visiting[id] = true
		defer delete(visiting, id)
		seen[id] = true
		ids = append(ids, id)
		var relations []*TaskRelation
		if err := s.Where("task_id = ? AND relation_kind = ?", id, RelationKindSubtask).Find(&relations); err != nil {
			return err
		}
		sort.Slice(relations, func(i, j int) bool { return relations[i].OtherTaskID < relations[j].OtherTaskID })
		for _, relation := range relations {
			if _, err := GetTaskByIDSimple(s, relation.OtherTaskID); err != nil {
				if IsErrTaskDoesNotExist(err) {
					continue
				}
				return err
			}
			if previous := parents[relation.OtherTaskID]; previous == 0 || id < previous {
				parents[relation.OtherTaskID] = id
			}
			if err := walk(relation.OtherTaskID, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return ids, parents, walk(root, 1)
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
	doc, err := html.Parse(strings.NewReader(body))
	return err == nil && taskTraceIsOutstandingDocument(doc)
}

func taskTraceTeamOutstandingNodes(body string) (*html.Node, []*html.Node) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil, nil
	}
	nodes := []*html.Node{}
	taskTraceWalk(doc, func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "li" && taskTraceAttribute(node, "data-id") != "" {
			nodes = append(nodes, node)
		}
	})
	return doc, nodes
}

func taskTraceTeamRenderDocument(doc *html.Node) string {
	if doc == nil {
		return ""
	}
	var body *html.Node
	taskTraceWalk(doc, func(node *html.Node) {
		if body == nil && node.Type == html.ElementNode && node.Data == "body" {
			body = node
		}
	})
	if body == nil {
		return ""
	}
	return taskTraceInnerHTML(body)
}

func taskTraceTeamOutstandingItems(body string) (map[string]string, []string) {
	items := map[string]string{}
	order := []string{}
	_, nodes := taskTraceTeamOutstandingNodes(body)
	for _, node := range nodes {
		id := taskTraceAttribute(node, "data-id")
		if _, exists := items[id]; !exists {
			order = append(order, id)
		}
		items[id] = taskTraceInnerHTML(node)
	}
	return items, order
}

func taskTraceTeamOutstandingHTML(items map[string]string, order []string) string {
	if len(items) == 0 {
		return ""
	}
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
	body.WriteString(`<h3 ` + taskTraceOutstandingTypeAttribute + `="` + taskTraceOutstandingType + `">TaskTrace 遗留事项清单</h3><ul>`)
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
	_, nodes := taskTraceTeamOutstandingNodes(body)
	for _, node := range nodes {
		id := taskTraceAttribute(node, "data-id")
		if value := taskTraceAttribute(node, "data-priority"); len(value) == 1 && value[0] >= '0' && value[0] <= '9' {
			priorities[id] = value
		}
	}
	return priorities
}

func taskTraceTeamOutstandingPriorityKey(nodeID, outstandingID string) string {
	return nodeID + ":" + outstandingID
}

func taskTraceTeamRememberOutstandingPriorities(binding *TaskTraceTeamBinding, snapshot TaskTraceTeamSnapshot) {
	if binding.OutstandingPriorities == nil {
		binding.OutstandingPriorities = map[string]int{}
	}
	for _, task := range snapshot.Tasks {
		for id, value := range taskTraceTeamOutstandingPriorities(task.Outstanding) {
			parsed, err := strconv.Atoi(value)
			if err == nil && parsed >= 0 && parsed <= 9 {
				binding.OutstandingPriorities[taskTraceTeamOutstandingPriorityKey(task.NodeID, id)] = parsed
			}
		}
	}
}

func taskTraceTeamRememberedOutstandingPriorities(binding *TaskTraceTeamBinding, nodeID string) map[string]string {
	result := map[string]string{}
	for key, value := range binding.OutstandingPriorities {
		prefix := nodeID + ":"
		if strings.HasPrefix(key, prefix) && value >= 0 && value <= 9 {
			result[strings.TrimPrefix(key, prefix)] = strconv.Itoa(value)
		}
	}
	return result
}

func taskTraceTeamApplyOutstandingPriorities(body string, priorities map[string]string) string {
	if len(priorities) == 0 {
		return body
	}
	doc, nodes := taskTraceTeamOutstandingNodes(body)
	for _, node := range nodes {
		priority, ok := priorities[taskTraceAttribute(node, "data-id")]
		if !ok {
			continue
		}
		updated := false
		for index := range node.Attr {
			if node.Attr[index].Key == "data-priority" {
				node.Attr[index].Val = priority
				updated = true
			}
		}
		if !updated {
			node.Attr = append(node.Attr, html.Attribute{Key: "data-priority", Val: priority})
		}
	}
	return taskTraceTeamRenderDocument(doc)
}

func taskTraceTeamStripOutstandingPriorities(body string) string {
	if !strings.Contains(body, "data-priority") {
		return body
	}
	doc, nodes := taskTraceTeamOutstandingNodes(body)
	for _, node := range nodes {
		attributes := node.Attr[:0]
		for _, attribute := range node.Attr {
			if attribute.Key != "data-priority" {
				attributes = append(attributes, attribute)
			}
		}
		node.Attr = attributes
	}
	return taskTraceTeamRenderDocument(doc)
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
		key := taskTraceTeamLocalAttachmentKey(nodeID, id)
		if binding.LocalAttachments[key] == 0 {
			binding.LocalAttachments[key] = attachment.ID
		}
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
		// Keep every source id even when multiple attachments have identical
		// bytes. HTML may reference any of them and every source must be
		// rewritten to the one local attachment selected above.
		result = append(result, TaskTraceTeamAttachment{ID: id, Name: attachment.File.Name, Mime: attachment.File.Mime, Size: uint64(len(content)), SourceTaskID: taskID, SourceAttachmentID: attachment.ID})
	}
	sort.Slice(result, func(i, j int) bool {
		return taskTraceTeamAttachmentSourceKey(result[i]) < taskTraceTeamAttachmentSourceKey(result[j])
	})
	return result, nil
}

func taskTraceTeamBuildSnapshot(s *xorm.Session, binding *TaskTraceTeamBinding, actor, device string) (TaskTraceTeamSnapshot, error) {
	if binding.NodeTasks == nil {
		binding.NodeTasks = map[string]int64{}
	}
	roots := []int64{binding.RootTaskID}
	for _, taskID := range binding.NodeTasks {
		if taskID != 0 && taskID != binding.RootTaskID {
			roots = append(roots, taskID)
		}
	}
	additionalRoots := roots[1:]
	sort.Slice(additionalRoots, func(i, j int) bool { return additionalRoots[i] < additionalRoots[j] })
	ids := []int64{}
	parents := map[int64]int64{}
	seen := map[int64]bool{}
	for _, root := range roots {
		subtreeIDs, subtreeParents, err := taskTraceTeamSubtree(s, root)
		if err != nil {
			return TaskTraceTeamSnapshot{}, err
		}
		for _, taskID := range subtreeIDs {
			if !seen[taskID] {
				seen[taskID] = true
				ids = append(ids, taskID)
			}
		}
		for child, parent := range subtreeParents {
			if previous := parents[child]; previous == 0 || parent < previous {
				parents[child] = parent
			}
		}
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
	base := make(map[string]TaskTraceTeamBase, len(binding.Base))
	for node, value := range binding.Base {
		value.Outstanding = taskTraceTeamStripOutstandingPriorities(value.Outstanding)
		base[node] = value
	}
	snapshot := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: actor, DeviceID: device, Updated: time.Now().UTC(), Avatar: taskTraceTeamAvatarDataURI(s, actor), ResolutionAcks: acks, Base: base, Tasks: []TaskTraceTeamTask{}}
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
	normalized.Base = nil
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
			if comment.Deleted {
				continue
			}
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
		sort.Slice(devices, func(i, j int) bool {
			if !devices[i].Updated.Equal(devices[j].Updated) {
				return devices[i].Updated.Before(devices[j].Updated)
			}
			if devices[i].DeviceID != devices[j].DeviceID {
				return devices[i].DeviceID < devices[j].DeviceID
			}
			return taskTraceTeamSnapshotHash(devices[i]) < taskTraceTeamSnapshotHash(devices[j])
		})
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
				if previous, ok := comments[comment.ID]; !ok || taskTraceTeamCommentEventSupersedes(comment, previous) {
					comments[comment.ID] = comment
				}
			}
			task.Comments = task.Comments[:0]
			for _, comment := range comments {
				task.Comments = append(task.Comments, comment)
			}
			sort.Slice(task.Comments, func(i, j int) bool {
				if !task.Comments[i].Created.Equal(task.Comments[j].Created) {
					return task.Comments[i].Created.Before(task.Comments[j].Created)
				}
				return task.Comments[i].ID < task.Comments[j].ID
			})
			attachments := map[string]TaskTraceTeamAttachment{}
			for _, attachment := range task.Attachments {
				attachments[taskTraceTeamAttachmentSourceKey(attachment)] = attachment
			}
			task.Attachments = task.Attachments[:0]
			for _, attachment := range attachments {
				task.Attachments = append(task.Attachments, attachment)
			}
			sort.Slice(task.Attachments, func(i, j int) bool {
				return taskTraceTeamAttachmentSourceKey(task.Attachments[i]) < taskTraceTeamAttachmentSourceKey(task.Attachments[j])
			})
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

// taskTraceTeamCommentEventSupersedes resolves concurrent snapshots of the
// same shared comment. A deletion must win a timestamp tie; otherwise an older
// actor snapshot can recreate a progress entry which the task owner deleted.
func taskTraceTeamCommentEventSupersedes(candidate, current TaskTraceTeamComment) bool {
	if candidate.Updated.After(current.Updated) {
		return true
	}
	if !candidate.Updated.Equal(current.Updated) {
		return false
	}
	if candidate.Deleted != current.Deleted {
		return candidate.Deleted
	}
	// Snapshot files are discovered through filesystem iteration, whose order is
	// not a synchronization contract. Use a stable tie-breaker so two edits with
	// SQLite's same-second timestamp cannot alternate between sync runs.
	candidateKey := strings.Join([]string{candidate.Body, strings.ToLower(candidate.Author), candidate.Created.UTC().Format(time.RFC3339Nano)}, "\x00")
	currentKey := strings.Join([]string{current.Body, strings.ToLower(current.Author), current.Created.UTC().Format(time.RFC3339Nano)}, "\x00")
	return candidateKey > currentKey
}

// taskTraceTeamReconcileLocalCommentEvents turns comments which disappeared
// since the previous snapshot into durable deletion events. Without these
// tombstones another member's older snapshot would recreate a deleted comment
// during the next merge. A restored comment receives a newer event timestamp so
// Undo can intentionally supersede the tombstone.
func taskTraceTeamReconcileLocalCommentEvents(current *TaskTraceTeamSnapshot, previous *TaskTraceTeamSnapshot, now time.Time) {
	if current == nil || previous == nil {
		return
	}
	previousTasks := taskTraceTeamTaskMap(*previous)
	for taskIndex := range current.Tasks {
		task := &current.Tasks[taskIndex]
		oldTask, exists := previousTasks[task.NodeID]
		if !exists {
			continue
		}
		currentByID := make(map[string]int, len(task.Comments))
		for index := range task.Comments {
			currentByID[task.Comments[index].ID] = index
		}
		for _, old := range oldTask.Comments {
			index, stillPresent := currentByID[old.ID]
			if old.Deleted {
				if !stillPresent {
					task.Comments = append(task.Comments, old)
					currentByID[old.ID] = len(task.Comments) - 1
					continue
				}
				if !task.Comments[index].Deleted && !task.Comments[index].Updated.After(old.Updated) {
					task.Comments[index].Updated = now
				}
				continue
			}
			if stillPresent {
				continue
			}
			old.Body = ""
			old.Deleted = true
			old.Updated = now
			task.Comments = append(task.Comments, old)
			currentByID[old.ID] = len(task.Comments) - 1
		}
	}
}

func taskTraceTeamLatestCommentEventSnapshot(snapshots []TaskTraceTeamSnapshot) TaskTraceTeamSnapshot {
	tasks := map[string]map[string]TaskTraceTeamComment{}
	for _, snapshot := range snapshots {
		for _, task := range snapshot.Tasks {
			if tasks[task.NodeID] == nil {
				tasks[task.NodeID] = map[string]TaskTraceTeamComment{}
			}
			for _, comment := range task.Comments {
				previous, exists := tasks[task.NodeID][comment.ID]
				if !exists || taskTraceTeamCommentEventSupersedes(comment, previous) {
					tasks[task.NodeID][comment.ID] = comment
				}
			}
		}
	}
	result := TaskTraceTeamSnapshot{Tasks: make([]TaskTraceTeamTask, 0, len(tasks))}
	for nodeID, comments := range tasks {
		task := TaskTraceTeamTask{NodeID: nodeID, Comments: make([]TaskTraceTeamComment, 0, len(comments))}
		for _, comment := range comments {
			task.Comments = append(task.Comments, comment)
		}
		result.Tasks = append(result.Tasks, task)
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
	case "description":
		return task.Description
	case "done":
		return strconv.FormatBool(task.Done)
	case "status":
		if task.Status != "" {
			return string(task.Status)
		}
		if task.Done {
			return string(TaskStatusDone)
		}
		return string(TaskStatusTodo)
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
	case "description":
		return base.Description
	case "done":
		return strconv.FormatBool(base.Done)
	case "status":
		if base.Status != "" {
			return string(base.Status)
		}
		if base.Done {
			return string(TaskStatusDone)
		}
		return string(TaskStatusTodo)
	}
	return ""
}

func taskTraceTeamNormalizeText(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	inWhitespace := false
	for _, r := range strings.ReplaceAll(value, "\r\n", "\n") {
		if unicode.IsSpace(r) {
			if !inWhitespace {
				normalized.WriteByte(' ')
				inWhitespace = true
			}
			continue
		}
		inWhitespace = false
		normalized.WriteRune(r)
	}
	return normalized.String()
}

func taskTraceTeamCanonicalHTML(value string, attachments []TaskTraceTeamAttachment) string {
	for _, attachment := range attachments {
		if attachment.ID == "" {
			continue
		}
		canonical := "tasktrace-team-attachment:" + attachment.ID
		for _, version := range []string{"v1", "v2"} {
			source := fmt.Sprintf("/api/%s/tasks/%d/attachments/%d", version, attachment.SourceTaskID, attachment.SourceAttachmentID)
			value = strings.ReplaceAll(value, source, canonical)
		}
	}
	doc, err := html.Parse(strings.NewReader("<div>" + value + "</div>"))
	if err != nil {
		return strings.TrimSpace(taskTraceTeamNormalizeText(value))
	}
	var root *html.Node
	taskTraceWalk(doc, func(node *html.Node) {
		if root == nil && node.Type == html.ElementNode && node.Data == "div" {
			root = node
		}
	})
	if root == nil {
		return strings.TrimSpace(taskTraceTeamNormalizeText(value))
	}
	var normalize func(*html.Node, bool)
	normalize = func(node *html.Node, preserveWhitespace bool) {
		if node.Type == html.ElementNode {
			preserveWhitespace = preserveWhitespace || node.Data == "pre" || node.Data == "code"
			attributes := node.Attr[:0]
			for _, attribute := range node.Attr {
				if attribute.Key == "data-priority" {
					continue
				}
				attributes = append(attributes, attribute)
			}
			node.Attr = attributes
			sort.Slice(node.Attr, func(i, j int) bool {
				if node.Attr[i].Namespace != node.Attr[j].Namespace {
					return node.Attr[i].Namespace < node.Attr[j].Namespace
				}
				if node.Attr[i].Key != node.Attr[j].Key {
					return node.Attr[i].Key < node.Attr[j].Key
				}
				return node.Attr[i].Val < node.Attr[j].Val
			})
		}
		for child := node.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == html.CommentNode {
				node.RemoveChild(child)
			} else {
				if child.Type == html.TextNode && !preserveWhitespace {
					child.Data = taskTraceTeamNormalizeText(child.Data)
				}
				normalize(child, preserveWhitespace)
			}
			child = next
		}
	}
	normalize(root, false)
	return strings.TrimSpace(taskTraceInnerHTML(root))
}

func taskTraceTeamCanonicalFieldValue(task TaskTraceTeamTask, field, value string) string {
	if field == "description" || strings.HasPrefix(field, "outstanding:") {
		return taskTraceTeamCanonicalHTML(value, task.Attachments)
	}
	if field == "title" {
		return strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	}
	return value
}

func taskTraceTeamConflictID(shareID, node, field string, options []TaskTraceTeamConflictOption) string {
	b, _ := json.Marshal(options)
	sum := sha256.Sum256(append([]byte(shareID+"\x00"+node+"\x00"+field+"\x00"), b...))
	return hex.EncodeToString(sum[:16])
}

func taskTraceTeamFindField(snapshots []TaskTraceTeamSnapshot, node, field string, base string, requiredResolution string, localTasks ...TaskTraceTeamTask) (string, []TaskTraceTeamConflictOption, bool) {
	type fieldValue struct {
		value  string
		actors []string
	}
	values := map[string]*fieldValue{}
	baseTask := TaskTraceTeamTask{}
	if len(localTasks) > 0 {
		baseTask = localTasks[0]
	} else {
		for _, snapshot := range snapshots {
			task, ok := taskTraceTeamTaskMap(snapshot)[node]
			if ok && taskTraceTeamFieldValue(task, field) == base {
				baseTask = task
				break
			}
		}
	}
	baseValue := taskTraceTeamCanonicalFieldValue(baseTask, field, base)
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
		canonical := taskTraceTeamCanonicalFieldValue(task, field, value)
		snapshotBaseCanonical := baseValue
		if snapshotBase, exists := snapshot.Base[node]; exists {
			snapshotBaseValue := taskTraceTeamBaseValue(snapshotBase, field)
			snapshotBaseCanonical = taskTraceTeamCanonicalFieldValue(task, field, snapshotBaseValue)
		}
		if canonical != snapshotBaseCanonical {
			if values[canonical] == nil {
				values[canonical] = &fieldValue{value: value}
			}
			values[canonical].actors = append(values[canonical].actors, snapshot.Actor)
		}
	}
	if len(values) == 0 {
		return base, nil, false
	}
	if len(values) == 1 {
		for _, grouped := range values {
			return grouped.value, nil, false
		}
	}
	options := make([]TaskTraceTeamConflictOption, 0, len(values))
	for _, grouped := range values {
		sort.Strings(grouped.actors)
		options = append(options, TaskTraceTeamConflictOption{Author: strings.Join(grouped.actors, "、"), Value: grouped.value})
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

func taskTraceTeamLocalResolutionValue(binding *TaskTraceTeamBinding, nodeID, value string) (string, error) {
	snapshots, err := taskTraceTeamReadSnapshots(binding)
	if err != nil {
		return "", err
	}
	rows := make([]struct {
		actor string
		task  TaskTraceTeamTask
	}, 0, len(snapshots))
	for _, snapshot := range snapshots {
		for _, task := range snapshot.Tasks {
			if task.NodeID == nodeID {
				rows = append(rows, struct {
					actor string
					task  TaskTraceTeamTask
				}{actor: snapshot.Actor, task: task})
			}
		}
	}
	return taskTraceTeamRewriteAttachments(value, binding.NodeTasks[nodeID], taskTraceTeamAllAttachments(rows), binding), nil
}

func taskTraceTeamApplyResolution(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, record taskTraceTeamResolutionRecord) error {
	taskID := binding.NodeTasks[record.NodeID]
	if taskID == 0 {
		return nil
	}
	base := binding.Base[record.NodeID]
	value := record.Value
	if record.Field == "description" || strings.HasPrefix(record.Field, "outstanding:") {
		var err error
		value, err = taskTraceTeamLocalResolutionValue(binding, record.NodeID, value)
		if err != nil {
			return err
		}
	}
	switch record.Field {
	case "title":
		base.Title = value
	case "description":
		base.Description = value
	case "done":
		base.Done, _ = strconv.ParseBool(value)
		if base.Done {
			base.Status = TaskStatusDone
		} else if base.Status == TaskStatusDone || base.Status == "" {
			base.Status = TaskStatusTodo
		}
	case "status":
		base.Status = TaskStatus(value)
		base.Done = base.Status == TaskStatusDone
	default:
		if strings.HasPrefix(record.Field, "outstanding:") {
			items, order := taskTraceTeamOutstandingItems(base.Outstanding)
			id := strings.TrimPrefix(record.Field, "outstanding:")
			if value == "" {
				delete(items, id)
			} else {
				items[id] = value
			}
			base.Outstanding = taskTraceTeamOutstandingHTML(items, order)
		}
	}
	if strings.HasPrefix(record.Field, "outstanding:") {
		binding.Base[record.NodeID] = base
		return taskTraceTeamUpsertOutstanding(s, a, taskID, base.Outstanding)
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
	if record.Field != "description" {
		base.Description = stored.Description
	}
	binding.Base[record.NodeID] = base
	return taskTraceTeamApplyTaskFields(s, a, taskID, base.Title, base.Description, base.Done, base.Status)
}

func taskTraceTeamApplyPendingResolutions(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, records map[string]taskTraceTeamResolutionRecord) error {
	if binding.ResolutionAcks == nil {
		binding.ResolutionAcks = map[string]string{}
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		record := records[key]
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
	var current *TaskComment
	for _, comment := range comments {
		if !taskTraceTeamIsOutstanding(comment.Comment) {
			continue
		}
		if current == nil {
			current = comment
			continue
		}
		// Only one canonical list belongs to a task. Old releases and concurrent
		// syncs could leave additional empty or stale list comments behind.
		if _, err := s.ID(comment.ID).NoAutoCondition().Delete(&TaskComment{}); err != nil {
			return err
		}
	}
	if strings.TrimSpace(body) == "" {
		if current != nil {
			_, err := s.ID(current.ID).NoAutoCondition().Delete(&TaskComment{})
			return err
		}
		return nil
	}
	if current != nil {
		if current.Comment == body {
			return nil
		}
		current.Comment = body
		return current.Update(s, a)
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
		if shared.Deleted {
			if existing := byID[shared.ID]; existing != nil && !existing.Updated.After(shared.Updated) {
				if _, err := s.ID(existing.ID).NoAutoCondition().Delete(&TaskComment{}); err != nil {
					return err
				}
				delete(byID, shared.ID)
			}
			continue
		}
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

func taskTraceTeamTaskOrderKey(snapshot TaskTraceTeamSnapshot, task TaskTraceTeamTask) string {
	metadata := task
	metadata.Comments = nil
	metadata.Attachments = nil
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return strings.Join([]string{task.NodeID, strings.ToLower(snapshot.Actor), snapshot.DeviceID}, "\x00")
	}
	return strings.Join([]string{strings.ToLower(snapshot.Actor), snapshot.DeviceID, string(encoded)}, "\x00")
}

// taskTraceTeamMissingTaskPlan merges every collaborator snapshot before it
// creates anything. A single "richest" snapshot is insufficient: a member can
// add a new child while another member still has more old nodes in their tree.
func taskTraceTeamMissingTaskPlan(binding *TaskTraceTeamBinding, snapshots []TaskTraceTeamSnapshot) ([]TaskTraceTeamTask, error) {
	type candidate struct {
		task TaskTraceTeamTask
		key  string
	}
	candidates := map[string]candidate{}
	for _, snapshot := range snapshots {
		for _, task := range snapshot.Tasks {
			if strings.TrimSpace(task.NodeID) == "" {
				return nil, errors.New("shared task contains an empty node id")
			}
			if binding.NodeTasks[task.NodeID] != 0 {
				continue
			}
			key := taskTraceTeamTaskOrderKey(snapshot, task)
			previous, exists := candidates[task.NodeID]
			if !exists || task.Updated.After(previous.task.Updated) || (task.Updated.Equal(previous.task.Updated) && key > previous.key) {
				candidates[task.NodeID] = candidate{task: task, key: key}
			}
		}
	}

	state := map[string]uint8{}
	depth := map[string]int{}
	plan := make([]TaskTraceTeamTask, 0, len(candidates))
	var visit func(string) (int, error)
	visit = func(node string) (int, error) {
		if state[node] == 1 {
			return 0, fmt.Errorf("shared task hierarchy contains a cycle at node %s", node)
		}
		if state[node] == 2 {
			return depth[node], nil
		}
		entry, exists := candidates[node]
		if !exists {
			if binding.NodeTasks[node] != 0 {
				return 0, nil
			}
			return 0, fmt.Errorf("shared task node %s refers to missing parent %s", node, node)
		}
		state[node] = 1
		level := 1
		parent := entry.task.ParentNode
		if parent == node {
			return 0, fmt.Errorf("shared task node %s cannot be its own parent", node)
		}
		if parent != "" {
			if binding.NodeTasks[parent] != 0 {
				level = 1
			} else if _, ok := candidates[parent]; ok {
				parentDepth, err := visit(parent)
				if err != nil {
					return 0, err
				}
				level = parentDepth + 1
			} else {
				return 0, fmt.Errorf("shared task node %s refers to missing parent %s", node, parent)
			}
		}
		if level > MaxTaskHierarchyDepth {
			return 0, fmt.Errorf("shared task hierarchy exceeds %d levels at node %s", MaxTaskHierarchyDepth, node)
		}
		state[node] = 2
		depth[node] = level
		plan = append(plan, entry.task)
		return level, nil
	}

	nodes := make([]string, 0, len(candidates))
	for node := range candidates {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if _, err := visit(node); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func taskTraceTeamValidateMissingTaskDepth(s *xorm.Session, binding *TaskTraceTeamBinding, plan []TaskTraceTeamTask) error {
	planned := make(map[string]TaskTraceTeamTask, len(plan))
	for _, task := range plan {
		planned[task.NodeID] = task
	}
	for _, task := range plan {
		chainLength := 1
		parent := task.ParentNode
		for parent != "" && binding.NodeTasks[parent] == 0 {
			parentTask, ok := planned[parent]
			if !ok {
				return fmt.Errorf("shared task node %s refers to missing parent %s", task.NodeID, parent)
			}
			chainLength++
			parent = parentTask.ParentNode
		}
		above := 0
		if parent != "" {
			var err error
			above, err = taskHierarchySpan(s, binding.NodeTasks[parent], RelationKindParenttask, 1)
			if err != nil {
				return err
			}
		}
		if above+chainLength > MaxTaskHierarchyDepth {
			return fmt.Errorf("shared task hierarchy exceeds %d levels at node %s", MaxTaskHierarchyDepth, task.NodeID)
		}
	}
	return nil
}

func taskTraceTeamCreateMissingTasks(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, snapshots []TaskTraceTeamSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	if binding.NodeTasks == nil {
		binding.NodeTasks = map[string]int64{}
	}
	plan, err := taskTraceTeamMissingTaskPlan(binding, snapshots)
	if err != nil {
		return err
	}
	if err := taskTraceTeamValidateMissingTaskDepth(s, binding, plan); err != nil {
		return err
	}
	for _, shared := range plan {
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

func taskTraceTeamAttachmentSourceKey(attachment TaskTraceTeamAttachment) string {
	return attachment.ID + ":" + strconv.FormatInt(attachment.SourceTaskID, 10) + ":" + strconv.FormatInt(attachment.SourceAttachmentID, 10)
}

func taskTraceTeamAllAttachments(rows []struct {
	actor string
	task  TaskTraceTeamTask
}) []TaskTraceTeamAttachment {
	bySource := map[string]TaskTraceTeamAttachment{}
	for _, row := range rows {
		for _, attachment := range row.task.Attachments {
			bySource[taskTraceTeamAttachmentSourceKey(attachment)] = attachment
		}
	}
	result := make([]TaskTraceTeamAttachment, 0, len(bySource))
	for _, attachment := range bySource {
		result = append(result, attachment)
	}
	sort.Slice(result, func(i, j int) bool {
		return taskTraceTeamAttachmentSourceKey(result[i]) < taskTraceTeamAttachmentSourceKey(result[j])
	})
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

func taskTraceTeamPruneMissingTaskMappings(s *xorm.Session, binding *TaskTraceTeamBinding) error {
	for node, taskID := range binding.NodeTasks {
		if _, err := GetTaskByIDSimple(s, taskID); err != nil {
			if !IsErrTaskDoesNotExist(err) {
				return err
			}
			if taskID == binding.RootTaskID {
				return errors.New("the collaborative root task was deleted; undo the deletion or import the shared task again")
			}
			delete(binding.NodeTasks, node)
			for key := range binding.LocalAttachments {
				if strings.HasPrefix(key, node+":") {
					delete(binding.LocalAttachments, key)
				}
			}
		}
	}
	return nil
}

func taskTraceTeamMergeBinding(s *xorm.Session, a web.Auth, state *taskTraceTeamState, binding *TaskTraceTeamBinding, actor string) error {
	if err := taskTraceTeamValidateBinding(binding, actor); err != nil {
		return err
	}
	if err := taskTraceTeamPruneMissingTaskMappings(s, binding); err != nil {
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
	taskTraceTeamRememberOutstandingPriorities(binding, local)
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
	taskTraceTeamReconcileLocalCommentEvents(&local, previousLocal, time.Now().UTC())
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
	// Keep each device snapshot during the three-way merge. One user may edit
	// different fields on two computers at the same time; collapsing both into
	// the newest whole-task snapshot would silently discard the older device's
	// independent field change. Comments and attachments are deduplicated below.
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
		localTask, hasLocalTask := taskTraceTeamTaskMap(local)[node]
		stored, err := GetTaskByIDSimple(s, taskID)
		if err != nil {
			return err
		}
		title, description, status := base.Title, base.Description, base.Status
		fields := []string{"title", "description", "status"}
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
			value, options, conflict := taskTraceTeamFindField(snapshots, node, field, taskTraceTeamBaseValue(base, field), required, localTask)
			if conflict {
				conflicts = append(conflicts, TaskTraceTeamConflict{ID: taskTraceTeamConflictID(binding.ShareID, node, field, options), ShareID: binding.ShareID, NodeID: node, TaskID: taskID, TaskTitle: stored.Title, Field: field, Base: taskTraceTeamBaseValue(base, field), Options: options})
				continue
			}
			switch field {
			case "title":
				title = value
			case "description":
				description = value
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
		outstanding := taskTraceTeamOutstandingHTML(outstandingItems, outstandingOrder)
		localPriorities := taskTraceTeamRememberedOutstandingPriorities(binding, node)
		if hasLocalTask {
			for id, value := range taskTraceTeamOutstandingPriorities(localTask.Outstanding) {
				localPriorities[id] = value
			}
		}
		outstanding = taskTraceTeamApplyOutstandingPriorities(outstanding, localPriorities)
		attachments := taskTraceTeamAllAttachments(rows)
		description = taskTraceTeamRewriteAttachments(description, taskID, attachments, binding)
		outstanding = taskTraceTeamRewriteAttachments(outstanding, taskID, attachments, binding)
		if status == "" {
			if base.Done {
				status = TaskStatusDone
			} else {
				status = TaskStatusTodo
			}
		}
		done := status == TaskStatusDone
		if stored.Title != title || stored.Description != description || stored.Done != done || stored.Status != status {
			if err := taskTraceTeamApplyTaskFields(s, a, taskID, title, description, done, status); err != nil {
				return err
			}
		}
		// This is deliberately idempotent: it also removes empty/duplicate list
		// comments left by older builds without producing another edit record.
		if err := taskTraceTeamUpsertOutstanding(s, a, taskID, outstanding); err != nil {
			return err
		}
		commentByID := map[string]TaskTraceTeamComment{}
		for _, row := range rows {
			for _, comment := range row.task.Comments {
				comment.Body = taskTraceTeamRewriteAttachments(comment.Body, taskID, row.task.Attachments, binding)
				if previous, ok := commentByID[comment.ID]; !ok || taskTraceTeamCommentEventSupersedes(comment, previous) {
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
	latestCommentEvents := taskTraceTeamLatestCommentEventSnapshot(snapshots)
	taskTraceTeamReconcileLocalCommentEvents(&final, &latestCommentEvents, time.Now().UTC())
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
			taskTraceTeamMemberAccessMu.Lock()
			resolved, err = taskTraceTeamGrantWindowsAccess(binding.Repository, member)
			taskTraceTeamMemberAccessMu.Unlock()
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
	release, err := taskTraceTeamAcquireShareLock(binding)
	if err != nil {
		return nil, err
	}
	defer release()
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
	status, err := taskTraceTeamStatusLockedFast(s, a, state)
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
		taskTraceTeamMemberAccessMu.Lock()
		resolved, grantErr := taskTraceTeamGrantWindowsAccess(root, member)
		taskTraceTeamMemberAccessMu.Unlock()
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
	status, err := taskTraceTeamStatusLockedFast(s, a, state)
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
	state.Bindings = append(state.Bindings, binding)
	importedBinding := &state.Bindings[len(state.Bindings)-1]
	release, err := taskTraceTeamAcquireShareLock(importedBinding)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := taskTraceTeamMergeBinding(s, a, &state, importedBinding, u.Username); err != nil {
		return nil, err
	}
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLockedFast(s, a, state)
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
		backupBytes, cloneErr := json.Marshal(binding)
		if cloneErr != nil {
			return nil, cloneErr
		}
		var backup TaskTraceTeamBinding
		if cloneErr = json.Unmarshal(backupBytes, &backup); cloneErr != nil {
			return nil, cloneErr
		}
		release, lockErr := taskTraceTeamAcquireShareLock(binding)
		if lockErr != nil {
			binding.LastError = lockErr.Error()
			continue
		}
		savepoint := fmt.Sprintf("tasktrace_team_sync_%d", i)
		eventCheckpoint := events.PendingCheckpoint(s)
		if _, err := s.Exec("SAVEPOINT " + savepoint); err != nil {
			release()
			return nil, fmt.Errorf("start team task synchronization: %w", err)
		}
		mergeErr := taskTraceTeamMergeBinding(s, a, &state, binding, u.Username)
		release()
		if mergeErr != nil {
			if _, rollbackErr := s.Exec("ROLLBACK TO SAVEPOINT " + savepoint); rollbackErr != nil {
				return nil, fmt.Errorf("rollback failed team task synchronization: %w", rollbackErr)
			}
			events.RollbackPendingTo(s, eventCheckpoint)
			if _, releaseErr := s.Exec("RELEASE SAVEPOINT " + savepoint); releaseErr != nil {
				return nil, fmt.Errorf("release failed team task synchronization: %w", releaseErr)
			}
			*binding = backup
			binding.LastError = mergeErr.Error()
			continue
		}
		if _, err := s.Exec("RELEASE SAVEPOINT " + savepoint); err != nil {
			return nil, fmt.Errorf("finish team task synchronization: %w", err)
		}
	}
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLockedFast(s, a, state)
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
	status, err := taskTraceTeamStatusLockedFast(s, a, state)
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
	release, err := taskTraceTeamAcquireShareLock(binding)
	if err != nil {
		return nil, err
	}
	defer release()
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
	status, err := taskTraceTeamStatusLockedFast(s, a, state)
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
	status, err := taskTraceTeamStatusLockedFast(s, a, state)
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
		profiles[taskTraceTeamProfileKey(member)] = TaskTraceTeamMemberProfile{Username: member}
	}
	snapshots, err := taskTraceTeamReadSnapshots(binding)
	if err == nil {
		for _, snapshot := range taskTraceTeamLatestActorSnapshots(snapshots) {
			key := taskTraceTeamProfileKey(snapshot.Actor)
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

func taskTraceTeamProfileKey(username string) string {
	return strings.ToLower(taskTraceTeamMembershipName(username))
}

func taskTraceTeamStoredProfiles(s *xorm.Session) ([]TaskTraceTeamMemberProfile, error) {
	accounts := []*user.User{}
	if err := s.Where("issuer = ?", taskTraceWindowsTeamIssuer).Find(&accounts); err != nil {
		return nil, err
	}
	profiles := make([]TaskTraceTeamMemberProfile, 0, len(accounts))
	for _, account := range accounts {
		email := strings.TrimSpace(account.Email)
		if strings.HasSuffix(strings.ToLower(email), "@tasktrace.invalid") {
			email = ""
		}
		profiles = append(profiles, TaskTraceTeamMemberProfile{
			Username: account.Username, AccountName: account.Subject,
			DisplayName: account.Name, Email: email,
		})
	}
	return profiles, nil
}

func taskTraceTeamMergeProfile(current, incoming TaskTraceTeamMemberProfile) TaskTraceTeamMemberProfile {
	if incoming.Username != "" {
		current.Username = incoming.Username
	}
	if incoming.AccountName != "" {
		current.AccountName = incoming.AccountName
	}
	if incoming.DisplayName != "" {
		current.DisplayName = incoming.DisplayName
	}
	if incoming.Email != "" {
		current.Email = incoming.Email
	}
	if incoming.Avatar != "" {
		current.Avatar = incoming.Avatar
	}
	return current
}

func taskTraceTeamStatusLocked(s *xorm.Session, a web.Auth, state taskTraceTeamState) (TaskTraceTeamStatus, error) {
	return taskTraceTeamStatusLockedWithAccessScan(s, a, state, true)
}

func taskTraceTeamStatusLockedFast(s *xorm.Session, a web.Auth, state taskTraceTeamState) (TaskTraceTeamStatus, error) {
	return taskTraceTeamStatusLockedWithAccessScan(s, a, state, false)
}

func taskTraceTeamStatusLockedWithAccessScan(s *xorm.Session, a web.Auth, state taskTraceTeamState, scanAccess bool) (TaskTraceTeamStatus, error) {
	u, err := user.GetFromAuth(a)
	if err != nil {
		return TaskTraceTeamStatus{}, err
	}
	root := taskTraceTeamRoot()
	repository := taskTraceTeamRepositoryInfoCached(root)
	if scanAccess {
		repository = taskTraceTeamRepositoryInfo(root)
	}
	status := TaskTraceTeamStatus{Enabled: taskTraceTeamEnabled(), Username: u.Username, Repository: repository, UnassignedMembers: []string{}, Bindings: []TaskTraceTeamBindingStatus{}, Conflicts: []TaskTraceTeamConflict{}, Notifications: []TaskTraceTeamNotification{}, Profiles: []TaskTraceTeamMemberProfile{}}
	status.UnassignedMembers = taskTraceTeamUnassignedMembers(state, status.Repository.Candidates, u.Username)
	profiles := map[string]TaskTraceTeamMemberProfile{
		taskTraceTeamProfileKey(u.Username): {Username: u.Username, DisplayName: u.Name, Email: u.Email, Avatar: taskTraceTeamAvatarDataURI(s, u.Username)},
	}
	for _, binding := range state.Bindings {
		manifest := TaskTraceTeamManifest{Owner: binding.Owner, Members: binding.Members}
		if manifestErr := taskTraceTeamReadJSON(filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json"), &manifest); manifestErr != nil {
			manifest = TaskTraceTeamManifest{Owner: binding.Owner, Members: binding.Members}
		}
		manifest.Members = taskTraceTeamNormalizeMembers(manifest.Members, manifest.Owner)
		ids := make([]int64, 0, len(binding.NodeTasks))
		for _, id := range binding.NodeTasks {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		linkPath := binding.Repository
		linkPaths := []string{}
		if binding.Owner == u.Username {
			// Only the main repository needs an ACL scan. Reading the same Windows
			// share permissions once per collaboration made a page refresh slower
			// as more shared tasks were added.
			info := taskTraceTeamRepositoryMetadata(binding.Repository)
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
			key := taskTraceTeamProfileKey(profile.Username)
			profiles[key] = taskTraceTeamMergeProfile(profiles[key], profile)
		}
	}
	storedProfiles, err := taskTraceTeamStoredProfiles(s)
	if err != nil {
		return TaskTraceTeamStatus{}, err
	}
	for _, profile := range storedProfiles {
		key := taskTraceTeamProfileKey(profile.Username)
		profiles[key] = taskTraceTeamMergeProfile(profiles[key], profile)
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
	if !taskTraceTeamEnabled() {
		return TaskTraceTeamStatus{Enabled: false}, nil
	}
	// State and shared manifests are replaced atomically. Reading a snapshot does
	// not need the mutation lock, and must remain available while a slow network
	// share or Windows administrator prompt is delaying a write operation.
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return TaskTraceTeamStatus{}, err
	}
	return taskTraceTeamStatusLocked(s, a, state)
}
