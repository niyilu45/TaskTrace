import {beforeEach, describe, expect, it, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'

import {tasksTeamShare, tasktraceTeamMembersAccessCreate, tasktraceTeamMembersSearch, tasktraceTeamStatus} from '@/client/generated'
import {useTasktraceTeamStore} from './tasktraceTeam'

vi.mock('@/client/generated', async (importOriginal) => ({
	...(await importOriginal<typeof import('@/client/generated')>()),
	tasktraceTeamMembersAccessCreate: vi.fn(),
	tasktraceTeamMembersSearch: vi.fn(),
	tasktraceTeamStatus: vi.fn(),
	tasksTeamShare: vi.fn(),
}))

beforeEach(() => {
	setActivePinia(createPinia())
	vi.clearAllMocks()
})

describe('TaskTrace team bindings', () => {
	it('finds a legacy root binding even when task_ids is empty', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [], owner: 'owner', members: ['teammate']}],
			conflicts: [],
			notifications: [],
		}

		expect(store.bindingForTask(42)?.share_id).toBe('shared')
	})

	it('finds descendants from task_ids', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [42, 43], owner: 'owner', members: ['owner', 'teammate']}],
			conflicts: [],
			notifications: [],
		}

		expect(store.bindingForTask(43)?.share_id).toBe('shared')
	})

	it('uses the current member task permission for editing', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			username: 'reader',
			bindings: [{
				share_id: 'shared',
				root_task_id: 42,
				task_ids: [42],
				owner: 'owner',
				members: ['owner', 'reader'],
				permission_targets: [{
					node_id: 'root',
					task_id: 42,
					kind: 'task',
					permissions: [
						{username: 'owner', read: true, write: true, owner: true},
						{username: 'reader', read: true, write: false},
					],
				}],
			}],
			conflicts: [],
			notifications: [],
		}

		expect(store.canWriteTask(42)).toBe(false)
	})

	it('keeps the collaboration owner writable for a legacy manifest', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			username: 'owner',
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [42], owner: 'owner', members: ['owner', 'reader']}],
			conflicts: [],
			notifications: [],
		}

		expect(store.canWriteTask(42)).toBe(true)
	})

	it('shares one deduplicated active member roster without retaining profile-only accounts', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			username: 'Current',
			bindings: [{
				share_id: 'shared', root_task_id: 42, task_ids: [42], owner: 'Owner', members: ['owner', 'Member'],
				permission_targets: [{kind: 'task', task_id: 42, permissions: [{username: 'Reader', read: true}]}],
			}],
			profiles: [{username: 'ProfileOnly'}],
			repository: {candidates: ['FolderUser']},
			unassigned_members: ['Unassigned'],
			conflicts: [], notifications: [],
		}

		expect(store.memberRoster.map(member => member.toLocaleLowerCase()).sort()).toEqual([
			'current', 'folderuser', 'member', 'owner', 'reader', 'unassigned',
		].sort())
	})

	it('uses persisted Windows identity details immediately after refresh', async () => {
		vi.mocked(tasktraceTeamStatus).mockResolvedValue({data: {
			enabled: true,
			username: 'current',
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [42], owner: 'current', members: ['654321']}],
			profiles: [{username: '654321', account_name: 'CHINA\\654321', display_name: '张三', email: 'zhangsan@example.com'}],
			conflicts: [],
			notifications: [],
		}} as unknown as Awaited<ReturnType<typeof tasktraceTeamStatus>>)

		const store = useTasktraceTeamStore()
		await store.refresh()

		expect(store.displayNameFor('654321')).toBe('张三')
		expect(store.identityTitleFor('654321')).toContain('zhangsan@example.com')
		expect(vi.mocked(tasktraceTeamMembersSearch).mock.calls.some(([request]) => request?.query?.q === '654321')).toBe(false)
	})

	it('persists a resolved identity for an existing teamData member', async () => {
		const unresolvedStatus = {
			enabled: true,
			username: 'current',
			repository: {candidates: ['CHINA\\654321']},
			bindings: [],
			profiles: [],
			conflicts: [],
			notifications: [],
		}
		const resolvedProfile = {username: '654321', account_name: 'CHINA\\654321', display_name: '张三', email: 'zhangsan@example.com'}
		vi.mocked(tasktraceTeamStatus).mockResolvedValue({data: unresolvedStatus} as unknown as Awaited<ReturnType<typeof tasktraceTeamStatus>>)
		vi.mocked(tasktraceTeamMembersSearch).mockResolvedValue({data: {candidates: [resolvedProfile]}} as unknown as Awaited<ReturnType<typeof tasktraceTeamMembersSearch>>)
		vi.mocked(tasktraceTeamMembersAccessCreate).mockResolvedValue({data: {...unresolvedStatus, profiles: [resolvedProfile]}} as unknown as Awaited<ReturnType<typeof tasktraceTeamMembersAccessCreate>>)

		const store = useTasktraceTeamStore()
		await store.refresh()

		await vi.waitFor(() => expect(tasktraceTeamMembersAccessCreate).toHaveBeenCalledWith(expect.objectContaining({
			body: expect.objectContaining({account_name: 'CHINA\\654321', display_name: '张三'}),
		})))
		expect(store.displayNameFor('654321')).toBe('张三')
	})

	it('does not let an older refresh hide a member saved by a newer request', async () => {
		let finishRefresh!: (value: Awaited<ReturnType<typeof tasktraceTeamStatus>>) => void
		vi.mocked(tasktraceTeamStatus).mockImplementation(() => new Promise(resolve => { finishRefresh = resolve }))
		vi.mocked(tasksTeamShare).mockResolvedValue({data: {
			enabled: true,
			username: 'owner',
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [42], owner: 'owner', members: ['owner', 'new-member']}],
			conflicts: [],
			notifications: [],
		}} as unknown as Awaited<ReturnType<typeof tasksTeamShare>>)

		const store = useTasktraceTeamStore()
		const staleRefresh = store.refresh()
		await store.share(42, ['new-member'])
		finishRefresh({data: {
			enabled: true,
			username: 'owner',
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [42], owner: 'owner', members: ['owner']}],
			conflicts: [],
			notifications: [],
		}} as unknown as Awaited<ReturnType<typeof tasktraceTeamStatus>>)
		await staleRefresh

		expect(store.bindingForTask(42)?.members).toContain('new-member')
	})
})
