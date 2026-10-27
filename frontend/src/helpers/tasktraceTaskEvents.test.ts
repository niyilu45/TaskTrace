import {describe, expect, it} from 'vitest'
import TaskModel from '@/models/task'
import {tasktraceDeletionScope} from './tasktraceTaskEvents'

describe('local task deletion scope', () => {
	it('removes known descendants and identifies their surviving parent without following other relations', () => {
		const parent = new TaskModel({id: 1})
		const child = new TaskModel({id: 2, relatedTasks: {parenttask: [parent]}})
		const grandchild = new TaskModel({id: 3, relatedTasks: {parenttask: [child]}})
		const peer = new TaskModel({id: 4, relatedTasks: {related: [child]}})
		parent.relatedTasks = {subtask: [child]}
		const scope = tasktraceDeletionScope(child, [parent, child, grandchild, peer])
		expect([...scope.ids].sort()).toEqual([2, 3])
		expect([...scope.parents]).toEqual([1])
	})

	it('terminates on corrupt cyclic references and preserves upstream single-item deletion', () => {
		const first = new TaskModel({id: 1})
		const second = new TaskModel({id: 2, relatedTasks: {subtask: [first]}})
		first.relatedTasks = {subtask: [second]}
		expect([...tasktraceDeletionScope(first, [first, second]).ids].sort()).toEqual([1, 2])
		expect([...tasktraceDeletionScope(first, [first, second], false).ids]).toEqual([1])
	})
})
