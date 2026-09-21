import {defineStore} from 'pinia'
import {computed, ref} from 'vue'

import {
	tasksTeamShare,
	tasktraceTeamConfigure,
	tasktraceTeamImport,
	tasktraceTeamMembersAccessCreate,
	tasktraceTeamMembersAccessDelete,
	tasktraceTeamMembersImport,
	tasktraceTeamMembersSearch,
	tasktraceTeamNotificationsRead,
	tasktraceTeamPermissionsConfigure,
	tasktraceTeamResolve,
	tasktraceTeamStatus,
	tasktraceTeamSync,
	type TaskTraceTeamBindingStatus,
	type TaskTraceTeamMemberCandidate,
	type TaskTraceTeamMemberImportResult,
	type TaskTraceTeamPermissionUpdate,
	type TaskTraceTeamResolution,
	type TaskTraceTeamStatus,
} from '@/client/generated'
import {overrideTeamLinkRepository} from '@/helpers/tasktraceTeam'

export const useTasktraceTeamStore = defineStore('tasktraceTeam', () => {
	const status = ref<TaskTraceTeamStatus>({enabled: false, bindings: [], conflicts: [], notifications: []})
	const loading = ref(false)
	const loaded = ref(false)
	let activeRead: Promise<TaskTraceTeamStatus> | null = null
	let pending = 0

	function apply(next?: TaskTraceTeamStatus) {
		if (next) status.value = next
		loaded.value = true
		return status.value
	}

	async function run(request: () => Promise<{data: TaskTraceTeamStatus}>, deduplicate = false) {
		if (deduplicate && activeRead) return activeRead
		pending++
		loading.value = true
		const operation = request().then(result => apply(result.data)).finally(() => {
			pending--
			loading.value = pending > 0
			if (activeRead === operation) activeRead = null
		})
		if (deduplicate) activeRead = operation
		return operation
	}

	async function refresh() {
		return run(() => tasktraceTeamStatus(), true)
	}

	async function sync() {
		return run(() => tasktraceTeamSync(), true)
	}

	async function share(taskId: number, members: string[]) {
		return run(() => tasksTeamShare({path: {task: taskId}, body: {members}}))
	}

	async function importLink(link: string, projectId: number, repository = '') {
		return run(() => tasktraceTeamImport({body: {link: overrideTeamLinkRepository(link, repository), project_id: projectId}}))
	}

	async function configure(shareId: string, notify: boolean) {
		return run(() => tasktraceTeamConfigure({body: {share_id: shareId, notify}}))
	}

	async function configurePermissions(shareId: string, taskId: number, outstandingId: string, permissions: TaskTraceTeamPermissionUpdate[]) {
		return run(() => tasktraceTeamPermissionsConfigure({body: {
			share_id: shareId,
			task_id: taskId,
			outstanding_id: outstandingId,
			permissions,
		}}))
	}

	async function resolve(shareId: string, resolutions: TaskTraceTeamResolution[]) {
		return run(() => tasktraceTeamResolve({body: {share_id: shareId, resolutions}}))
	}

	async function dismissNotifications(ids: string[] = []) {
		return run(() => tasktraceTeamNotificationsRead({body: {ids}}))
	}

	async function searchMembers(query: string, signal?: AbortSignal): Promise<TaskTraceTeamMemberCandidate[]> {
		const result = await tasktraceTeamMembersSearch({query: {q: query}, signal})
		return result.data.candidates ?? []
	}

	async function grantMember(accountName: string) {
		return run(() => tasktraceTeamMembersAccessCreate({body: {account_name: accountName}}))
	}

	async function removeMember(accountName: string) {
		return run(() => tasktraceTeamMembersAccessDelete({path: {member: accountName}}))
	}

	async function importMembers(link: string): Promise<TaskTraceTeamMemberImportResult> {
		pending++
		loading.value = true
		try {
			const result = await tasktraceTeamMembersImport({body: {link}})
			if (result.data.status) apply(result.data.status)
			return result.data
		} finally {
			pending--
			loading.value = pending > 0
		}
	}

	function bindingForTask(taskId: number): TaskTraceTeamBindingStatus | undefined {
		return status.value.bindings?.find(binding => binding.root_task_id === taskId || binding.task_ids?.includes(taskId))
	}

	function permissionForTask(taskId: number) {
		const binding = bindingForTask(taskId)
		const target = binding?.permission_targets?.find(item => item.kind === 'task' && item.task_id === taskId)
		const username = status.value.username?.toLowerCase()
		return target?.permissions?.find(permission => permission.username?.toLowerCase() === username)
	}

	function canWriteTask(taskId: number) {
		const binding = bindingForTask(taskId)
		if (!binding) return true
		const permission = permissionForTask(taskId)
		if (permission) return permission.write === true
		return binding.owner?.toLowerCase() === status.value.username?.toLowerCase()
	}

	const conflictCount = computed(() => status.value.conflicts?.length ?? 0)
	const notificationCount = computed(() => status.value.notifications?.length ?? 0)
	const activityCount = computed(() => conflictCount.value + notificationCount.value)

	return {status, loading, loaded, conflictCount, notificationCount, activityCount, refresh, sync, share, importLink, configure, configurePermissions, resolve, dismissNotifications, searchMembers, grantMember, removeMember, importMembers, bindingForTask, permissionForTask, canWriteTask}
})
