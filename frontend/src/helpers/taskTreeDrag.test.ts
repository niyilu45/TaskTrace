import {describe, expect, it} from 'vitest'
import type {ITask} from '@/modelTypes/ITask'
import {taskMovePlan, taskSubtreeIds} from './taskTreeDrag'

function task(id: number, parentId = 0): ITask {
	return {
		id,
		relatedTasks: parentId ? {parenttask: [{id: parentId}]} : {},
	} as ITask
}

describe('task tree drag', () => {
	const tasks = [task(1), task(2, 1), task(3, 2), task(4), task(5, 4)]

	it('moves the selected child without selecting its parent', () => {
		expect(taskMovePlan(tasks, 2, 5, 'before')).toEqual({parentId: 4, beforeTaskId: 5})
		expect(taskSubtreeIds(2, tasks)).toEqual(new Set([2, 3]))
	})

	it('keeps sibling placement deterministic after removing the dragged task', () => {
		const siblings = [task(1), task(2), task(3)]
		expect(taskMovePlan(siblings, 1, 2, 'after')).toEqual({parentId: 0, beforeTaskId: 3})
		expect(taskMovePlan(siblings, 3, 2, 'after')).toEqual({parentId: 0, beforeTaskId: 0})
	})

	it('rejects dropping a task into its own subtree', () => {
		expect(taskMovePlan(tasks, 1, 3, 'inside')).toBeNull()
		expect(taskMovePlan(tasks, 2, 3, 'after')).toBeNull()
	})
})
