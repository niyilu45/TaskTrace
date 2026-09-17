import {beforeEach, describe, expect, it, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'
import {tasktraceUndoRead, tasktraceUndoCreate} from '@/client/generated'
import {useTasktraceUndoStore} from './tasktraceUndo'
import {undoInProgress, undoPendingWrites, undoViewVersion} from '@/helpers/tasktraceUndo'
const projects = vi.hoisted(() => ({loadAllProjects: vi.fn(async () => {})}))
vi.mock('@/client/generated', () => ({tasktraceUndoRead: vi.fn(), tasktraceUndoCreate: vi.fn()}))
vi.mock('@/stores/projects', () => ({useProjectStore: () => projects}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
const read = vi.mocked(tasktraceUndoRead)
const undo = vi.mocked(tasktraceUndoCreate)
beforeEach(() => {
	setActivePinia(createPinia())
	vi.clearAllMocks()
	undoInProgress.value = false; undoPendingWrites.value = 0; undoViewVersion.value = 0
	read.mockResolvedValue({data: {id: 4, label: '修改任务', count: 1}} as never)
})

describe('TaskTrace undo state', () => {
	it('refreshes clean views only after a successful server undo', async () => {
		const store = useTasktraceUndoStore()
		await store.refresh()
		undo.mockImplementationOnce(async () => {
			expect(undoInProgress.value).toBe(true)
			expect(undoViewVersion.value).toBe(0)
			read.mockResolvedValue({data: {id: 0, label: '', count: 0}} as never)
			return {data: {id: 0, label: '', count: 0}} as never
		})
		await store.undo()
		expect(undo).toHaveBeenCalledWith({body: {id: 4}})
		expect(undoViewVersion.value).toBe(1)
		expect(projects.loadAllProjects).toHaveBeenCalledTimes(1)
		expect(undoInProgress.value).toBe(false)
		expect(store.canUndo).toBe(false)
	})

	it('preserves the page and reports a conflict instead of overwriting newer data', async () => {
		const store = useTasktraceUndoStore()
		await store.refresh()
		undo.mockRejectedValueOnce({code: 4094, detail: '内容已修改，请刷新后重试。'})
		await store.undo()
		expect(undoViewVersion.value).toBe(0)
		expect(projects.loadAllProjects).not.toHaveBeenCalled()
		expect(store.error).toContain('内容已修改')
		expect(undoInProgress.value).toBe(false)
	})

	it('does not submit undo while another change is being saved', async () => {
		const store = useTasktraceUndoStore()
		await store.refresh()
		undoPendingWrites.value = 1
		await store.undo()
		expect(undo).not.toHaveBeenCalled()
		expect(undoViewVersion.value).toBe(0)
	})
})
