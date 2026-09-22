import {beforeEach, describe, expect, it, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'
import {watch} from 'vue'
import {tasktraceTeamStatus, tasktraceTeamSync, tasktraceTeamMembersSearch, tasksTeamShare} from '@/client/generated'
import {useTasktraceTeamStore} from './tasktraceTeam'

vi.mock('@/client/generated', async (original) => ({
	...(await original<typeof import('@/client/generated')>()),
	tasktraceTeamStatus: vi.fn(), tasktraceTeamSync: vi.fn(), tasktraceTeamMembersSearch: vi.fn(), tasksTeamShare: vi.fn(),
}))
const snapshot = () => ({enabled: true, username: 'me', bindings: [], notifications: [], conflicts: [], profiles: [{username: 'me', display_name: '本人'}]})
beforeEach(() => {
	setActivePinia(createPinia())
	vi.resetAllMocks()
	vi.mocked(tasktraceTeamMembersSearch).mockResolvedValue({data: {candidates: []}} as never)
	vi.mocked(tasktraceTeamStatus).mockImplementation(async () => ({data: snapshot()}) as never)
	vi.mocked(tasktraceTeamSync).mockImplementation(async () => ({data: snapshot()}) as never)
})
describe('collaboration refresh work', () => {
	it('waits for a member save before starting a newer status read', async () => {
		let finish!: (value: never) => void
		vi.mocked(tasksTeamShare).mockImplementationOnce(() => new Promise<never>(resolve => { finish = resolve }))
		const store = useTasktraceTeamStore()
		const save = store.share(1, ['other'])
		const read = store.refresh(true)
		await Promise.resolve()
		expect(tasktraceTeamStatus).not.toHaveBeenCalled()
		const next = {...snapshot(), bindings: [{share_id: 'new', members: ['me', 'other']}]}
		vi.mocked(tasktraceTeamStatus).mockResolvedValue({data: next} as never)
		finish({data: next} as never)
		await Promise.all([save, read])
		expect(store.status.bindings?.[0].share_id).toBe('new')
		expect(tasktraceTeamStatus).toHaveBeenCalledTimes(1)
	})
	it('coalesces a burst of component mounts even when reads finish between mounts', async () => {
		const store = useTasktraceTeamStore()
		for (let i = 0; i < 20; i++) await store.refresh()
		expect(tasktraceTeamStatus).toHaveBeenCalledTimes(1)
	})
	it('retains reactive status when 20 fresh reads contain no changes', async () => {
		const store = useTasktraceTeamStore()
		await store.refresh()
		const changed = vi.fn()
		const stop = watch(() => store.status, changed, {flush: 'sync'})
		for (let i = 0; i < 20; i++) await store.refresh(true)
		stop()
		expect(changed).not.toHaveBeenCalled()
	})
	it('does not turn an explicit synchronization into an in-flight status read', async () => {
		let finish!: (value: never) => void
		vi.mocked(tasktraceTeamStatus).mockImplementationOnce(() => new Promise<never>(resolve => { finish = resolve }))
		const store = useTasktraceTeamStore()
		const read = store.refresh()
		const sync = store.sync()
		await Promise.resolve()
		finish({data: snapshot()} as never)
		await Promise.all([read, sync])
		expect(tasktraceTeamSync).toHaveBeenCalledTimes(1)
	})
	it('retries unresolved background identities after a cooldown instead of every refresh', async () => {
		let now = 10000
		const clock = vi.spyOn(Date, 'now').mockImplementation(() => now)
		try {
			vi.mocked(tasktraceTeamStatus).mockImplementation(async () => ({data: {...snapshot(), repository: {candidates: ['missing-user']}}}) as never)
			const store = useTasktraceTeamStore()
			for (let i = 0; i < 20; i++) { await store.refresh(true); await Promise.resolve() }
			expect(tasktraceTeamMembersSearch).toHaveBeenCalledTimes(1)
			now += 300001
			await store.refresh(true)
			expect(tasktraceTeamMembersSearch).toHaveBeenCalledTimes(2)
		} finally { clock.mockRestore() }
	})
	it('does not replace a saved membership with an older status response', async () => {
		let finish!: (value: never) => void
		vi.mocked(tasktraceTeamStatus).mockImplementationOnce(() => new Promise<never>(resolve => { finish = resolve }))
		const store = useTasktraceTeamStore()
		const read = store.refresh()
		vi.mocked(tasksTeamShare).mockResolvedValue({data: {...snapshot(), bindings: [{share_id: 'new', members: ['me', 'other']}]}} as never)
		await store.share(1, ['other'])
		finish({data: snapshot()} as never)
		await read
		expect(store.status.bindings?.[0].share_id).toBe('new')
	})
})
