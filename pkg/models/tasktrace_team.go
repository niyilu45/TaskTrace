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

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"github.com/google/uuid"
	"xorm.io/xorm"
)

const taskTraceTeamSchema = 1

var taskTraceTeamMu sync.Mutex

type TaskTraceTeamRepositoryInfo struct {
	Path       string   `json:"path"`
	Computer   string   `json:"computer,omitempty"`
	Candidates []string `json:"candidates"`
	Shared     bool     `json:"shared"`
}

type TaskTraceTeamManifest struct {
	Schema    int       `json:"schema"`
	ShareID   string    `json:"share_id"`
	RootNode  string    `json:"root_node"`
	Owner     string    `json:"owner"`
	Members   []string  `json:"members"`
	TokenHash string    `json:"token_hash"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
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
	ShareID    string                  `json:"share_id"`
	Owner      string                  `json:"owner"`
	Members    []string                `json:"members"`
	RootTaskID int64                   `json:"root_task_id"`
	TaskIDs    []int64                 `json:"task_ids"`
	Link       string                  `json:"link"`
	Notify     bool                    `json:"notify"`
	LastSync   time.Time               `json:"last_sync,omitempty"`
	LastError  string                  `json:"last_error,omitempty"`
	Conflicts  []TaskTraceTeamConflict `json:"conflicts"`
}

type TaskTraceTeamNotification struct {
	ID        string    `json:"id"`
	ShareID   string    `json:"share_id"`
	Actor     string    `json:"actor"`
	TaskTitle string    `json:"task_title"`
	Created   time.Time `json:"created"`
}

type TaskTraceTeamStatus struct {
	Enabled       bool                         `json:"enabled"`
	Username      string                       `json:"username"`
	Repository    TaskTraceTeamRepositoryInfo  `json:"repository"`
	Bindings      []TaskTraceTeamBindingStatus `json:"bindings"`
	Conflicts     []TaskTraceTeamConflict      `json:"conflicts"`
	Notifications []TaskTraceTeamNotification  `json:"notifications"`
}

type taskTraceTeamLink struct {
	Schema     int    `json:"schema"`
	Repository string `json:"repository"`
	ShareID    string `json:"share_id"`
	Secret     string `json:"secret"`
}

var taskTraceTeamMarker = regexp.MustCompile(`<!--tasktrace-team:([A-Za-z0-9_-]+)-->`)
var taskTraceTeamOutstandingItem = regexp.MustCompile(`(?s)<li[^>]*data-id="([^"]+)"[^>]*>(.*?)</li>`)

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

func taskTraceTeamRepositoryInfo(root string) TaskTraceTeamRepositoryInfo {
	info := TaskTraceTeamRepositoryInfo{Path: root, Candidates: []string{}}
	_ = taskTraceTeamReadJSON(filepath.Join(root, "repository-info.json"), &info)
	if info.Path == "" {
		info.Path = root
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
		member = strings.TrimSpace(member)
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
	b, _ := json.Marshal(taskTraceTeamLink{Schema: taskTraceTeamSchema, Repository: repository, ShareID: shareID, Secret: secret})
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
		binding.LocalAttachments[taskTraceTeamLocalAttachmentKey(nodeID, id)] = attachment.ID
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
	acks := map[string]string{}
	for key, value := range binding.ResolutionAcks {
		acks[key] = value
	}
	snapshot := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: actor, DeviceID: device, Updated: time.Now().UTC(), ResolutionAcks: acks, Tasks: []TaskTraceTeamTask{}}
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
		shared := TaskTraceTeamTask{NodeID: node, ParentNode: parentNode, Title: task.Title, Description: task.Description, Done: task.Done, Status: task.Status, Updated: task.Updated, Comments: []TaskTraceTeamComment{}, Attachments: attachments}
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

func taskTraceTeamSnapshotHash(snapshot TaskTraceTeamSnapshot) string {
	copy := snapshot
	copy.Updated = time.Time{}
	b, _ := json.Marshal(copy)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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
				key := fmt.Sprintf("%d:%d", attachment.SourceTaskID, attachment.SourceAttachmentID)
				attachments[key] = attachment
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

func taskTraceTeamMergeComments(s *xorm.Session, a web.Auth, taskID int64, remote []TaskTraceTeamComment) error {
	var local []*TaskComment
	if err := s.Where("task_id = ?", taskID).Find(&local); err != nil {
		return err
	}
	byID := map[string]*TaskComment{}
	for _, comment := range local {
		if marker, ok := taskTraceTeamReadMarker(comment.Comment); ok {
			byID[marker.ID] = comment
		}
	}
	sort.Slice(remote, func(i, j int) bool { return remote[i].Created.Before(remote[j].Created) })
	for _, shared := range remote {
		body := taskTraceTeamAddMarker(shared.Body, shared.ID, shared.Author)
		if existing := byID[shared.ID]; existing != nil {
			if existing.Comment != body && shared.Updated.After(existing.Updated) {
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
	bySource := map[string]TaskTraceTeamAttachment{}
	for _, row := range rows {
		for _, attachment := range row.task.Attachments {
			key := fmt.Sprintf("%d:%d", attachment.SourceTaskID, attachment.SourceAttachmentID)
			bySource[key] = attachment
		}
	}
	result := make([]TaskTraceTeamAttachment, 0, len(bySource))
	for _, attachment := range bySource {
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
	localHash := taskTraceTeamSnapshotHash(local)
	if err := taskTraceTeamWriteSnapshot(binding, local); err != nil {
		return err
	}
	snapshots, err := taskTraceTeamReadSnapshots(binding)
	if err != nil {
		return err
	}
	snapshots = taskTraceTeamLatestActorSnapshots(snapshots)
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
		rows := allByNode[node]
		if len(rows) == 0 {
			continue
		}
		base := binding.Base[node]
		stored, err := GetTaskByIDSimple(s, taskID)
		if err != nil {
			return err
		}
		title, description, done, status, outstanding := base.Title, base.Description, base.Done, base.Status, base.Outstanding
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
		latest := rows[0].task
		for _, row := range rows[1:] {
			if row.task.Updated.After(latest.Updated) {
				latest = row.task
			}
		}
		attachments := taskTraceTeamAllAttachments(rows)
		description = taskTraceTeamRewriteAttachments(latest.Description, taskID, attachments, binding)
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
		if err := taskTraceTeamMergeComments(s, a, taskID, comments); err != nil {
			return err
		}
		binding.Base[node] = TaskTraceTeamBase{Title: title, Description: description, Done: done, Status: status, Outstanding: outstanding}
	}
	binding.Conflicts = conflicts
	binding.LastSync = time.Now().UTC()
	binding.LastError = ""
	if binding.Notify && binding.LastSnapshotHash != "" && binding.LastSnapshotHash != localHash {
		rootTitle := "团队任务"
		if root, err := GetTaskByIDSimple(s, binding.RootTaskID); err == nil {
			rootTitle = root.Title
		}
		for _, member := range binding.Members {
			if strings.EqualFold(member, actor) {
				continue
			}
			notice := TaskTraceTeamNotification{ID: uuid.NewString(), ShareID: binding.ShareID, Actor: actor, TaskTitle: rootTitle, Created: time.Now().UTC()}
			path := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "notifications", strings.ToLower(member), notice.ID+".json")
			_ = taskTraceTeamWriteJSON(path, &notice)
		}
	}
	binding.LastSnapshotHash = localHash
	return nil
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
	members := taskTraceTeamNormalizeMembers(request.Members, u.Username)
	if len(members) < 2 {
		return nil, errors.New("at least one other team member is required")
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	for _, existing := range state.Bindings {
		if existing.RootTaskID == request.TaskID {
			return nil, errors.New("this task is already shared")
		}
	}
	secret, err := taskTraceTeamRandomSecret()
	if err != nil {
		return nil, err
	}
	shareID := uuid.NewString()
	root := taskTraceTeamRoot()
	info := taskTraceTeamRepositoryInfo(root)
	binding := TaskTraceTeamBinding{ShareID: shareID, Repository: root, Secret: secret, Owner: u.Username, Members: members, RootTaskID: request.TaskID, NodeTasks: map[string]int64{}, Base: map[string]TaskTraceTeamBase{}, ResolutionAcks: map[string]string{}, LocalAttachments: map[string]int64{}, Notify: true}
	snapshot, err := taskTraceTeamBuildSnapshot(s, &binding, u.Username, state.DeviceID)
	if err != nil {
		return nil, err
	}
	if len(snapshot.Tasks) == 0 {
		return nil, errors.New("task subtree is empty")
	}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: shareID, RootNode: snapshot.Tasks[0].NodeID, Owner: u.Username, Members: members, TokenHash: taskTraceTeamTokenHash(secret), Created: time.Now().UTC(), Updated: time.Now().UTC()}
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
	var manifest TaskTraceTeamManifest
	if err := taskTraceTeamReadJSON(filepath.Join(taskTraceTeamShareDir(link.Repository, link.ShareID), "manifest.json"), &manifest); err != nil {
		return nil, fmt.Errorf("open team repository: %w", err)
	}
	if manifest.ShareID != link.ShareID || manifest.TokenHash != taskTraceTeamTokenHash(link.Secret) {
		return nil, errors.New("the task link is invalid or has been revoked")
	}
	isMember := false
	for _, member := range manifest.Members {
		if strings.EqualFold(member, u.Username) {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, errors.New("this task was not shared with the current Windows user")
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	if taskTraceTeamFindBinding(&state, link.ShareID) != nil {
		return nil, errors.New("this team task has already been imported")
	}
	binding := TaskTraceTeamBinding{ShareID: link.ShareID, Repository: link.Repository, Secret: link.Secret, Owner: manifest.Owner, Members: manifest.Members, ProjectID: request.ProjectID, NodeTasks: map[string]int64{}, Base: map[string]TaskTraceTeamBase{}, ResolutionAcks: map[string]string{}, LocalAttachments: map[string]int64{}, Notify: true}
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
	if err := taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(link.Repository, link.ShareID), "manifest.json"), &manifest); err != nil {
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

func taskTraceTeamNotifications(binding *TaskTraceTeamBinding, username string) []TaskTraceTeamNotification {
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
			result = append(result, notice)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Created.After(result[j].Created) })
	return result
}

func taskTraceTeamStatusLocked(s *xorm.Session, a web.Auth, state taskTraceTeamState) (TaskTraceTeamStatus, error) {
	u, err := user.GetFromAuth(a)
	if err != nil {
		return TaskTraceTeamStatus{}, err
	}
	root := taskTraceTeamRoot()
	status := TaskTraceTeamStatus{Enabled: taskTraceTeamEnabled(), Username: u.Username, Repository: taskTraceTeamRepositoryInfo(root), Bindings: []TaskTraceTeamBindingStatus{}, Conflicts: []TaskTraceTeamConflict{}, Notifications: []TaskTraceTeamNotification{}}
	for _, binding := range state.Bindings {
		ids := make([]int64, 0, len(binding.NodeTasks))
		for _, id := range binding.NodeTasks {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		linkPath := binding.Repository
		if binding.Owner == u.Username {
			info := taskTraceTeamRepositoryInfo(binding.Repository)
			linkPath = info.Path
		}
		row := TaskTraceTeamBindingStatus{ShareID: binding.ShareID, Owner: binding.Owner, Members: binding.Members, RootTaskID: binding.RootTaskID, TaskIDs: ids, Link: taskTraceTeamEncodeLink(linkPath, binding.ShareID, binding.Secret), Notify: binding.Notify, LastSync: binding.LastSync, LastError: binding.LastError, Conflicts: binding.Conflicts}
		status.Bindings = append(status.Bindings, row)
		status.Conflicts = append(status.Conflicts, binding.Conflicts...)
		status.Notifications = append(status.Notifications, taskTraceTeamNotifications(&binding, u.Username)...)
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
