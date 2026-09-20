import {describe, it, expect} from 'vitest'
import {groupProgressTasks, latestProgressDate, progressTaskPeople, recentProgressTaskIds, visibleProgressRows, type ProgressTask} from './projectProgress'
const task = (id: number, parents: number[] = []): ProgressTask => ({id, title: String(id), related_tasks: {parenttask: parents.map(id => ({id}))}})
describe('project progress grouping', () => {
 it('keeps descendants together, including completed children', () => {
  const groups = groupProgressTasks([task(1), {...task(2, [1]), done: true}, task(3, [2]), task(4)])
  expect(groups.map(group => group.rows.map(row => [row.task.id, row.depth]))).toEqual([[[1, 0], [2, 1], [3, 2]], [[4, 0]]])
 })
 it('retains every task once with cycles, multiple parents and external parents', () => {
  const groups = groupProgressTasks([task(1, [2]), task(2, [1]), task(3, [99]), task(4, [1, 3])])
  const ids = groups.flatMap(group => group.rows.map(row => row.task.id))
  expect(ids.sort()).toEqual([1, 2, 3, 4])
 })
})

describe('overview hierarchy visibility', () => {
 const rows = groupProgressTasks([task(1), {...task(2, [1]), done: true}, task(3, [2]), task(4, [1]), task(5, [4])])[0].rows
 it('collapses only descendants, leaving sibling branches visible', () => {
  expect(visibleProgressRows(rows, 'all', '', new Set([2])).map(row => row.task.id)).toEqual([1, 2, 4, 5])
  expect(visibleProgressRows(rows, 'all', '', new Set([1])).map(row => row.task.id)).toEqual([1])
 })
 it('retains completed ancestors of pending children', () => {
  expect(visibleProgressRows(rows, 'pending', '', new Set()).map(row => row.task.id)).toEqual([1, 2, 3, 4, 5])
  expect(visibleProgressRows(rows, 'done', '', new Set()).map(row => row.task.id)).toEqual([1, 2])
 })
	it('reveals the whole ancestor chain for search without losing saved collapse', () => {
  const collapsed = new Set([1, 2])
  expect(visibleProgressRows(rows, 'all', '3', collapsed).map(row => [row.task.id, row.depth])).toEqual([[1, 0], [2, 1], [3, 2]])
  expect(visibleProgressRows(rows, 'all', '', collapsed).map(row => row.task.id)).toEqual([1])
	})
	it('keeps only matching tasks and their ancestor chains without sibling branches', () => {
		expect(visibleProgressRows(rows, 'all', '', new Set(), new Set([3])).map(row => row.task.id)).toEqual([1, 2, 3])
		expect(visibleProgressRows(rows, 'all', '', new Set(), new Set([5])).map(row => row.task.id)).toEqual([1, 4, 5])
		expect(visibleProgressRows(rows, 'all', '', new Set(), new Set([2, 5])).map(row => row.task.id)).toEqual([1, 2, 4, 5])
	})
	it('combines recent progress matches with status and search filters', () => {
		expect(visibleProgressRows(rows, 'pending', '5', new Set(), new Set([3, 5])).map(row => row.task.id)).toEqual([1, 4, 5])
		expect(visibleProgressRows(rows, 'done', '', new Set(), new Set([3, 5]))).toEqual([])
	})
})

describe('project progress people', () => {
	it('uses creator and assignees for personal tasks without duplicates', () => {
		const item = {...task(1), created_by: {username: 'Alice'}, assignees: [{username: 'alice'}, {username: 'Bob'}]}
		expect(progressTaskPeople(item)).toEqual(['Alice', 'Bob'])
	})
	it('uses the original owner and remote assignees for collaborative tasks', () => {
		const item = {...task(1), created_by: {username: 'local-importer'}, assignees: [{username: 'local-importer'}]}
		const binding = {
			owner: 'Owner',
			permission_targets: [{
				kind: 'task' as const,
				task_id: 1,
				permissions: [
					{username: 'Owner', owner: true},
					{username: 'Assignee', assignee: true},
					{username: 'Reader', read: true},
				],
			}],
		}
		expect(progressTaskPeople(item, binding)).toEqual(['Owner', 'Assignee'])
	})
	it('attributes legacy tasks without people metadata to the current user', () => {
		expect(progressTaskPeople(task(1), undefined, 'CurrentUser')).toEqual(['CurrentUser'])
	})
})

describe('recent project progress', () => {
	const today = new Date(2026, 8, 19, 15, 30)
	it('uses inclusive local calendar days and excludes future progress', () => {
		const matches = recentProgressTaskIds({1: '2026-09-19', 2: '2026-09-13', 3: '2026-09-12', 4: '2026-09-20'}, 7, today)
		expect([...matches]).toEqual([1, 2])
	})
	it('works across month and year boundaries', () => {
		expect([...recentProgressTaskIds({1: '2025-12-31', 2: '2025-12-30'}, 2, new Date(2026, 0, 1))]).toEqual([1])
		expect([...recentProgressTaskIds({1: '2024-02-29', 2: '2024-02-28'}, 2, new Date(2024, 2, 1))]).toEqual([1])
	})
	it('ignores invalid dates and invalid day counts', () => {
		const dates = {1: '2026-02-30', 2: '日期未知', 3: '2026-09-19'}
		expect([...recentProgressTaskIds(dates, 7, today)]).toEqual([3])
		expect(recentProgressTaskIds(dates, 0, today).size).toBe(0)
	})
	it('uses the latest daily progress on or before today', () => {
		expect(latestProgressDate([
			{daily: false, date: '2026-09-19'},
			{daily: true, date: '2026-09-20'},
			{daily: true, date: '2026-09-18'},
			{daily: true, date: '2026-09-17'},
		], today)).toBe('2026-09-18')
	})
})
