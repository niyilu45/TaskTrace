import {beforeEach, describe, expect, it, vi} from 'vitest'
import {tasksRead} from '@/client/generated'
import {taskHierarchySpan, assertCanAddSubtask, TASK_DEPTH_MESSAGE} from './taskHierarchyDepth'
vi.mock('@/client/generated', () => ({tasksRead: vi.fn()}))
describe('task hierarchy preflight', () => {
	beforeEach(() => vi.clearAllMocks())
	it('allows adding level five and blocks level six', async () => {
		vi.mocked(tasksRead).mockImplementation(async ({path}) => ({data: {id: path.task, related_tasks: {parenttask: path.task > 1 ? [{id: path.task - 1}] : []}}}) as never)
		expect(await taskHierarchySpan(1)).toBe(1)
		expect(await taskHierarchySpan(5)).toBe(5)
		await expect(assertCanAddSubtask(4)).resolves.toBeUndefined()
		await expect(assertCanAddSubtask(5)).rejects.toThrow(TASK_DEPTH_MESSAGE)
	})
	it('checks the longest parent path and traverses subtrees in the other direction', async () => {
		vi.mocked(tasksRead).mockImplementation(async ({path}) => ({data: {related_tasks: {parenttask: path.task > 1 ? [{id: 1}, {id: path.task - 1}] : [], subtask: path.task < 5 ? [{id: path.task + 1}] : []}}}) as never)
		expect(await taskHierarchySpan(5)).toBe(5)
		expect(await taskHierarchySpan(1, 'subtask')).toBe(5)
	})
	it('fails closed on read errors and bounds legacy cycles', async () => {
		vi.mocked(tasksRead).mockRejectedValueOnce(new Error('offline'))
		await expect(assertCanAddSubtask(1)).rejects.toThrow('offline')
		vi.mocked(tasksRead).mockImplementation(async () => ({data: {related_tasks: {parenttask: [{id: 1}]}}}) as never)
		expect(await taskHierarchySpan(1)).toBe(5)
	})
})