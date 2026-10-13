import {beforeEach, describe, expect, it, vi} from 'vitest'
import {useTasktraceTeamSync} from './useTasktraceTeamSync'

const mocks = vi.hoisted(() => ({sync: vi.fn()}))
vi.mock('@/stores/tasktraceTeam', () => ({useTasktraceTeamStore: () => mocks}))
vi.mock('@/message', () => ({getErrorText: (error: Error) => error.message}))
beforeEach(() => { mocks.sync.mockReset() })

describe('project manual collaboration sync', () => {
	it('submits immediately, ignores repeat clicks while busy, and reports all completed bindings', async () => {
		let finish!: (value: unknown) => void
		mocks.sync.mockImplementation(() => new Promise(resolve => {finish = resolve}))
		const view = useTasktraceTeamSync()
		const first = view.syncNow()
		expect(view.syncing.value).toBe(true)
		expect(view.syncMessage.value).toContain('正在')
		await view.syncNow()
		expect(mocks.sync).toHaveBeenCalledTimes(1)
		finish({enabled: true, bindings: [{root_task_id: 1}, {root_task_id: 2}]})
		await first
		expect(view.syncMessage.value).toContain('同步完成，共 2 项')
		expect(view.syncing.value).toBe(false)
	})

	it('lists per-task errors even for a successful response, along with pending conflicts', async () => {
		mocks.sync.mockResolvedValue({enabled: true, bindings: [
			{root_task_id: 1, root_task_title: '共享任务甲', last_error: 'read member updates: denied'},
			{root_task_id: 2, conflicts: [{id: 'conflict'}]},
		]})
		const view = useTasktraceTeamSync()
		await view.syncNow()
		expect(view.syncMessage.value).toContain('1 / 2')
		expect(view.syncMessage.value).not.toContain('同步完成')
		expect(view.syncDetails.value).toEqual(['共享任务甲：read member updates: denied', '有 1 项冲突待处理，请打开通知查看差异。'])
	})

	it('keeps a missing-title task identifiable and clears stale errors on retry', async () => {
		mocks.sync.mockResolvedValueOnce({enabled: true, bindings: [{root_task_id: 8, last_error: 'offline'}]})
		const view = useTasktraceTeamSync()
		await view.syncNow()
		expect(view.syncDetails.value[0]).toBe('任务 #8：offline')
		mocks.sync.mockResolvedValueOnce({enabled: true, bindings: [{root_task_id: 8}]})
		await view.syncNow()
		expect(view.syncDetails.value).toEqual([])
		expect(view.syncMessage.value).toContain('同步完成')
	})

	it('allows retry after request errors', async () => {
		mocks.sync.mockRejectedValueOnce(new Error('connection timed out'))
		const view = useTasktraceTeamSync()
		await view.syncNow()
		expect(view.syncMessage.value).toContain('connection timed out')
		expect(view.syncing.value).toBe(false)
		mocks.sync.mockResolvedValue({enabled: true, bindings: [{root_task_id: 1}]})
		await view.syncNow()
		expect(view.syncMessage.value).toContain('同步完成')
	})

	it.each([undefined, false])('does not report success when sync is unavailable (%s)', async enabled => {
		mocks.sync.mockResolvedValue({enabled, bindings: []})
		const view = useTasktraceTeamSync()
		await view.syncNow()
		expect(view.syncMessage.value).toContain('不可用')
	})

	it('identifies an empty collaboration list', async () => {
		mocks.sync.mockResolvedValue({enabled: true, bindings: null})
		const view = useTasktraceTeamSync()
		await view.syncNow()
		expect(view.syncMessage.value).toBe('当前没有需要同步的协作任务。')
	})
})
