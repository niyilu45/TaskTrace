import {defineStore} from 'pinia'
import {computed, ref} from 'vue'
import equal from 'fast-deep-equal'

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
import {
	mergeTeamMemberCandidates,
	teamMemberDisplayName,
	teamMemberKey,
	teamMemberVerification,
} from '@/helpers/tasktraceTeamMembers'

export const useTasktraceTeamStore = defineStore('tasktraceTeam', () => {
	const status = ref<TaskTraceTeamStatus>({enabled: false, bindings: [], conflicts: [], notifications: []})
	const loading = ref(false)
	const loaded = ref(false)
	const directoryProfiles = ref<Record<string, TaskTraceTeamMemberCandidate>>({})
	let activeRead: Promise<TaskTraceTeamStatus> | null = null
	let activeSync: Promise<TaskTraceTeamStatus> | null = null
	const mutations = new Set<Promise<TaskTraceTeamStatus>>()
	let lastReadAt = -Infinity
	const profileRetryAfter = new Map<string, number>()
	let pending = 0
	let requestSequence = 0
	let appliedSequence = 0
	const hydratingProfiles = new Set<string>()

	function apply(next?: TaskTraceTeamStatus, sequence = ++requestSequence) {
		if (sequence < appliedSequence) return status.value
		appliedSequence = sequence
		if (next) {
			if (!equal(status.value, next)) status.value = next
			lastReadAt = Date.now()
			rememberMemberProfiles((next.profiles ?? []).map(profile => ({
				username: profile.username || '',
				account_name: profile.account_name || profile.username || '',
				display_name: profile.display_name,
				email: profile.email,
			})))
		}
		loaded.value = true
		void hydrateMemberProfiles()
		return status.value
	}

	async function run(request: () => Promise<{data: TaskTraceTeamStatus}>, kind: 'read' | 'sync' | 'write' = 'write'): Promise<TaskTraceTeamStatus> {
		// A status read started during a write must see the completed mutation.
		if (kind === 'read' && mutations.size) {
			await Promise.allSettled([...mutations])
			return run(request, kind)
		}
		if (kind === 'read' && activeRead) return activeRead
		if (kind === 'sync' && activeSync) return activeSync
		if (kind !== 'read') { lastReadAt = -Infinity; activeRead = null }
		const sequence = ++requestSequence
		pending++
		loading.value = true
		const operation = Promise.resolve().then(request).then(result => apply(result.data, sequence)).finally(() => {
			mutations.delete(operation)
			pending--
			loading.value = pending > 0
			if (activeRead === operation) activeRead = null
			if (activeSync === operation) activeSync = null
		})
		if (kind !== 'read') mutations.add(operation)
		if (kind === 'read') activeRead = operation
		if (kind === 'sync') activeSync = operation
		return operation
	}

	async function refresh(force = false) {
		if (!force && loaded.value && Date.now() - lastReadAt < 2000 && pending === 0) return status.value
		return run(() => tasktraceTeamStatus(), 'read')
	}

	async function sync() {
		return run(() => tasktraceTeamSync(), 'sync')
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

	async function searchMembers(query: string, signal?: AbortSignal, quick = false): Promise<TaskTraceTeamMemberCandidate[]> {
		const result = await tasktraceTeamMembersSearch({
			query: {q: query},
			signal,
			headers: quick ? {'X-TaskTrace-Quick': true} : undefined,
		})
		const candidates = result.data.candidates ?? []
		rememberMemberProfiles(candidates)
		return candidates
	}

	function rememberMemberProfiles(candidates: TaskTraceTeamMemberCandidate[]) {
		const next = {...directoryProfiles.value}
		let changed = false
		for (const candidate of candidates) {
			const key = teamMemberKey(candidate.account_name || candidate.username || candidate.email)
			if (!key) continue
			const merged = mergeTeamMemberCandidates([next[key]].filter(Boolean) as TaskTraceTeamMemberCandidate[], [candidate])[0]
			if (!equal(next[key], merged)) { next[key] = merged; changed = true }
		}
		if (changed) directoryProfiles.value = next
	}

	function memberProfile(username: string): TaskTraceTeamMemberCandidate {
		const key = teamMemberKey(username)
		return directoryProfiles.value[key] || {
			username: key || username,
			account_name: username,
		}
	}

	function displayNameFor(username: string) {
		return teamMemberDisplayName(memberProfile(username))
	}

	function identityTitleFor(username: string) {
		return teamMemberVerification(memberProfile(username))
	}

	function avatarFor(username: string) {
		const key = teamMemberKey(username)
		return status.value.profiles?.find(profile => teamMemberKey(profile.username) === key)?.avatar || ''
	}

	function uniqueMembers(values: Array<string | null | undefined>) {
		const members = new Map<string, string>()
		for (const value of values) {
			const member = value?.trim()
			const key = teamMemberKey(member)
			if (member && key && !members.has(key)) members.set(key, member)
		}
		return [...members.values()]
	}

	function hasKnownTeamDataAccess(member: string) {
		const key = teamMemberKey(member)
		if (!key || key === teamMemberKey(status.value.username)) return false
		return [
			...(status.value.repository?.candidates ?? []),
			...(status.value.unassigned_members ?? []),
			...(status.value.bindings ?? []).flatMap(binding => [binding.owner, ...(binding.members ?? [])]),
		].some(candidate => teamMemberKey(candidate) === key)
	}

	function memberAccessOptions(member: TaskTraceTeamMemberCandidate | string, elevate = false) {
		const candidate = typeof member === 'string' ? memberProfile(member) : member
		const accountName = candidate.account_name || candidate.username
		if (!accountName) throw new Error('缺少 Windows 账户名，无法设置 teamData 权限。')
		return {
			body: {
				account_name: accountName,
				username: candidate.username,
				display_name: candidate.display_name,
				email: candidate.email,
			},
			headers: elevate ? {'X-TaskTrace-Elevate': true as const} : undefined,
		}
	}

	async function hydrateMemberProfiles() {
		for (const member of memberRoster.value) {
			const key = teamMemberKey(member)
			const profile = directoryProfiles.value[key]
			const hasDirectoryIdentity = Boolean(profile?.email || (profile?.display_name && teamMemberKey(profile.display_name) !== key))
			if (!key || hydratingProfiles.has(key) || hasDirectoryIdentity || Date.now() < (profileRetryAfter.get(key) ?? 0)) continue
			// Failed/background directory lookups must not restart on every status poll.
			// Explicit user searches still run immediately.
			profileRetryAfter.set(key, Date.now() + 300_000)
			hydratingProfiles.add(key)
			void searchMembers(key)
				.then(async candidates => {
					const candidate = candidates.find(item => teamMemberKey(item.account_name || item.username || item.email) === key)
					if (!candidate || !hasKnownTeamDataAccess(member)) return
					const sequence = ++requestSequence
					const result = await tasktraceTeamMembersAccessCreate(memberAccessOptions(candidate))
					apply(result.data, sequence)
				})
				.catch(() => undefined)
				.finally(() => hydratingProfiles.delete(key))
		}
	}

	async function grantMember(member: TaskTraceTeamMemberCandidate | string, elevate = false) {
		return run(() => tasktraceTeamMembersAccessCreate(memberAccessOptions(member, elevate)))
	}

	async function removeMember(accountName: string, elevate = false) {
		return run(() => tasktraceTeamMembersAccessDelete({
			path: {member: accountName},
			headers: elevate ? {'X-TaskTrace-Elevate': true} : undefined,
		}))
	}

	async function importMembers(link: string): Promise<TaskTraceTeamMemberImportResult> {
		const sequence = ++requestSequence
		pending++
		loading.value = true
		try {
			const result = await tasktraceTeamMembersImport({body: {link}})
			if (result.data.status) apply(result.data.status, sequence)
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
		const username = teamMemberKey(status.value.username)
		return target?.permissions?.find(permission => teamMemberKey(permission.username) === username)
	}

	function canWriteTask(taskId: number) {
		const binding = bindingForTask(taskId)
		if (!binding) return true
		const permission = permissionForTask(taskId)
		if (permission) return permission.write === true
		return teamMemberKey(binding.owner) === teamMemberKey(status.value.username)
	}

	async function configureAssignees(shareId: string, taskId: number, assignees: string[]) {
		return run(() => tasktraceTeamPermissionsConfigure({body: {
			share_id: shareId,
			task_id: taskId,
			assignees,
		}}))
	}

	const conflictCount = computed(() => status.value.conflicts?.length ?? 0)
	const notificationCount = computed(() => status.value.notifications?.length ?? 0)
	const activityCount = computed(() => conflictCount.value + notificationCount.value)
	const memberRoster = computed(() => {
		const members = new Map<string, string>()
		const add = (value?: string | null) => {
			const username = value?.trim()
			if (!username) return
			const key = teamMemberKey(username)
			if (!members.has(key)) members.set(key, username)
		}
		add(status.value.username)
		for (const binding of status.value.bindings ?? []) {
			add(binding.owner)
			for (const member of binding.members ?? []) add(member)
			for (const target of binding.permission_targets ?? []) {
				for (const permission of target.permissions ?? []) add(permission.username)
			}
		}
		for (const member of status.value.repository?.candidates ?? []) add(member)
		for (const member of status.value.unassigned_members ?? []) add(member)
		return [...members.values()].sort((left, right) => left.localeCompare(right, 'zh-CN'))
	})

	return {status, loading, loaded, conflictCount, notificationCount, activityCount, memberRoster, refresh, sync, share, importLink, configure, configurePermissions, configureAssignees, resolve, dismissNotifications, searchMembers, grantMember, removeMember, importMembers, bindingForTask, permissionForTask, canWriteTask, memberProfile, displayNameFor, identityTitleFor, avatarFor, uniqueMembers, memberKey: teamMemberKey}
})
