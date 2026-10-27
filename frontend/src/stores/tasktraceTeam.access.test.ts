import {beforeEach, describe, expect, it, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'
import {tasktraceTeamMembersAccessCreate, tasktraceTeamPermissionsConfigure, tasktraceTeamMembersImport, tasksTeamShare, tasktraceTeamStatus} from '@/client/generated'
import {useTasktraceTeamStore} from './tasktraceTeam'

vi.mock('@/client/generated', async original => ({
	...(await original<typeof import('@/client/generated')>()),
	tasktraceTeamMembersAccessCreate: vi.fn(), tasktraceTeamPermissionsConfigure: vi.fn(), tasktraceTeamMembersImport: vi.fn(), tasksTeamShare: vi.fn(), tasktraceTeamStatus: vi.fn(),
	tasktraceTeamMembersSearch: vi.fn(async () => ({data: {candidates: []}})),
}))
const snapshot = () => ({enabled: true, username: 'owner', repository: {shared: true, candidates: [], members: [], pending_members: []}, bindings: [], conflicts: [], notifications: [], profiles: [{username: 'owner', display_name: 'Owner'}]})
const saved = () => ({...snapshot(), repository: {shared: true, candidates: ['CHINA\\member'], members: [{account_name: 'CHINA\\member', access: 'write'}], pending_members: []}, profiles: [...snapshot().profiles, {username: 'member', account_name: 'CHINA\\member', display_name: '新成员'}]})
const shareRequired = {status: 409, detail: 'Windows file sharing confirmation required'}
const adminRequired = {status: 403, detail: 'Windows administrator authorization required'}
beforeEach(() => {
	setActivePinia(createPinia())
	vi.clearAllMocks()
})

describe('team access synchronization', () => {
	it('requests an actual Windows permission rescan when opening member management', async () => {
		vi.mocked(tasktraceTeamStatus).mockResolvedValueOnce({data: snapshot()} as never)
		const store = useTasktraceTeamStore()
		await store.refresh(true, true)
		expect(tasktraceTeamStatus).toHaveBeenCalledWith(expect.objectContaining({headers: {'X-TaskTrace-Refresh-Access': true}}))
	})

	it('keeps successful link imports and retries permission failures with consent', async () => {
		const originalConfirm = window.confirm
		window.confirm = vi.fn(() => true)
		try {
			vi.mocked(tasktraceTeamMembersImport).mockResolvedValueOnce({data: {
				status: snapshot(), added: ['existing'], skipped_self: ['owner'], failed: [
					{username: 'member', reason: 'Windows file sharing confirmation required'},
					{username: 'missing-account', reason: 'Windows account not found'},
				],
			}} as never)
			vi.mocked(tasktraceTeamMembersAccessCreate).mockRejectedValueOnce(shareRequired).mockResolvedValueOnce({data: saved()} as never)
			const store = useTasktraceTeamStore()
			const result = await store.importMembers('link')
			expect(result.added).toEqual(['existing', 'member'])
			expect(result.failed?.map(item => item.username)).toEqual(['missing-account'])
			expect(result.skipped_self).toEqual(['owner'])
			expect(vi.mocked(tasktraceTeamMembersAccessCreate).mock.calls[1][0]?.headers).toEqual({'X-TaskTrace-Create-Share': true})
			expect(store.maximumAccessFor('member')).toBe('write')
			expect(store.loading).toBe(false)
		} finally { window.confirm = originalConfirm }
	})

	it('applies a newly granted member immediately and ignores an earlier status read', async () => {
		let finish!: (value: never) => void
		vi.mocked(tasktraceTeamStatus).mockImplementationOnce(() => new Promise<never>(resolve => {finish = resolve}))
		vi.mocked(tasktraceTeamMembersAccessCreate).mockResolvedValueOnce({data: saved()} as never)
		const store = useTasktraceTeamStore()
		const older = store.refresh()
		await store.grantMemberWithConsent({username: 'member', account_name: 'CHINA\\member', display_name: '新成员'})
		expect(store.memberRoster).toContain('CHINA\\member')
		expect(store.maximumAccessFor('member')).toBe('write')
		expect(store.displayNameFor('member')).toBe('新成员')
		finish({data: snapshot()} as never)
		await older
		expect(store.memberRoster).toContain('CHINA\\member')
	})

	it('reconciles reported permission gaps even for known task members, once per concurrent entry', async () => {
		const store = useTasktraceTeamStore()
		store.status = {...snapshot(), repository: {...snapshot().repository, pending_members: [{account_name: 'CHINA\\member', access: 'read'}]}, bindings: [{members: ['member']}]}
		vi.mocked(tasktraceTeamMembersAccessCreate).mockResolvedValueOnce({data: saved()} as never)
		await Promise.all([store.reconcileMemberAccess(), store.reconcileMemberAccess()])
		expect(tasktraceTeamMembersAccessCreate).toHaveBeenCalledTimes(1)
		expect(tasktraceTeamMembersAccessCreate).toHaveBeenCalledWith(expect.objectContaining({body: expect.objectContaining({account_name: 'CHINA\\member', access: 'read'})}))
		await store.reconcileMemberAccess()
		expect(tasktraceTeamMembersAccessCreate).toHaveBeenCalledTimes(1)
	})

	it('passes both explicit approvals to task sharing without clearing members on a declined retry', async () => {
		const originalConfirm = window.confirm
		const confirm = vi.fn(() => true)
		window.confirm = confirm
		try {
			vi.mocked(tasksTeamShare).mockRejectedValueOnce(shareRequired).mockRejectedValueOnce(adminRequired).mockResolvedValueOnce({data: saved()} as never)
			const store = useTasktraceTeamStore()
			await store.share(42, ['member'])
			expect(vi.mocked(tasksTeamShare).mock.calls[2][0]?.headers).toEqual({'X-TaskTrace-Create-Share': true, 'X-TaskTrace-Elevate': true})
			confirm.mockReturnValue(false)
			vi.mocked(tasksTeamShare).mockRejectedValueOnce(adminRequired)
			expect(await store.share(42, ['another'])).toBe(false)
			expect(store.maximumAccessFor('member')).toBe('write')
		} finally { window.confirm = originalConfirm }
	})

	it('retries task permission saves with administrator consent', async () => {
		const originalConfirm = window.confirm
		const confirm = vi.fn(() => true)
		window.confirm = confirm
		try {
			vi.mocked(tasktraceTeamPermissionsConfigure).mockRejectedValueOnce(adminRequired).mockResolvedValueOnce({data: saved()} as never)
			const store = useTasktraceTeamStore()
			await store.configurePermissions('share', 42, '', [{username: 'member', read: true, write: true}])
			expect(vi.mocked(tasktraceTeamPermissionsConfigure).mock.calls[1][0]?.headers).toEqual({'X-TaskTrace-Elevate': true})
		} finally { window.confirm = originalConfirm }
	})
})
