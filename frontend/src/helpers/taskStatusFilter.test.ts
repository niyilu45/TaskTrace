import {describe, it, expect} from 'vitest'
import {allTaskStatuses, legacyTaskStatusSelection, taskMatchesStatuses} from './taskStatusFilter'
import {groupProgressTasks, visibleProgressRows, type ProgressTask} from './projectProgress'

describe('task status selection', () => {
	it('keeps legacy defaults without merging todo and doing', () => {
		expect(legacyTaskStatusSelection('pending')).toEqual(['to-do', 'doing', 'hold'])
		expect(legacyTaskStatusSelection('all')).toEqual(allTaskStatuses)
		expect(taskMatchesStatuses({done: false}, ['to-do'])).toBe(true)
		expect(taskMatchesStatuses({status: 'doing'}, ['to-do'])).toBe(false)
		expect(taskMatchesStatuses({status: 'hold'}, ['doing'])).toBe(false)
		expect(taskMatchesStatuses({done: true, status: 'doing'}, ['doing'])).toBe(false)
		expect(taskMatchesStatuses({done: true, status: 'doing'}, ['done'])).toBe(true)
	})
	it('retains only ancestor context for a matched status, including empty selections', () => {
		const tasks: ProgressTask[] = [
			{id: 1, title: 'Paused parent', status: 'hold'},
			{id: 2, title: 'Doing child', status: 'doing', related_tasks: {parenttask: [{id: 1}]}},
			{id: 3, title: 'Todo sibling', status: 'to-do', related_tasks: {parenttask: [{id: 1}]}},
		]
		const rows = groupProgressTasks(tasks)[0].rows
		expect(visibleProgressRows(rows, ['doing'], '', new Set()).map(row => row.task.id)).toEqual([1, 2])
		expect(visibleProgressRows(rows, ['to-do'], '', new Set()).map(row => row.task.id)).toEqual([1, 3])
		expect(visibleProgressRows(rows, ['doing', 'to-do'], '', new Set()).map(row => row.task.id)).toEqual([1, 2, 3])
		expect(visibleProgressRows(rows, [], '', new Set())).toEqual([])
	})
})
