import {describe, it, expect} from 'vitest'
import TaskModel from '@/models/task'
import {groupOverviewTasks} from './overviewTasks'
const task = (id: number, projectId: number, parent?: number) => new TaskModel({id, projectId, relatedTasks: parent ? {parenttask: [{id: parent}]} : {}})
describe('overview project hierarchy', () => {
	it('groups projects and keeps completed ancestors as context without counting them', () => {
		const groups = groupOverviewTasks([task(3, 1, 2), task(4, 2), task(2, 1, 1)], [task(1, 1)])
		expect(groups.map(group => [group.projectId, group.count])).toEqual([[1, 2], [2, 1]])
		expect(groups[0].rows.map(row => [row.task.id, row.depth, row.context, row.hasChildren])).toEqual([[1, 0, true, true], [2, 1, false, true], [3, 2, false, false]])
	})
	it('keeps cross-project relations in their own project and handles cycles without duplicates', () => {
		const groups = groupOverviewTasks([task(1, 1, 2), task(2, 2, 1), task(3, 1, 4), task(4, 1, 3)], [])
		expect(groups.flatMap(group => group.rows.map(row => row.task.id)).sort()).toEqual([1, 2, 3, 4])
		expect(groups[1].rows[0].depth).toBe(0)
	})
})