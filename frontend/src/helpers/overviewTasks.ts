import type {ITask} from '@/modelTypes/ITask'
import {groupProgressTasks} from './projectProgress'

export function groupOverviewTasks(matches: ITask[], ancestors: ITask[]) {
	const matching = new Set(matches.map(task => task.id))
	const projects = new Map<number, Map<number, ITask>>()
	for (const task of [...matches, ...ancestors]) {
		if (!projects.has(task.projectId)) projects.set(task.projectId, new Map())
		const group = projects.get(task.projectId)!
		if (!group.has(task.id)) group.set(task.id, task)
	}
	return [...projects].map(([projectId, tasks]) => {
		const groups = groupProgressTasks([...tasks.values()].map(task => ({id: task.id, related_tasks: {parenttask: (task.relatedTasks?.parenttask || []).map(parent => ({id: parent.id}))}})))
		const rows = groups.flatMap(group => group.rows).map((row, index, rows) => ({
			task: tasks.get(row.task.id)!, depth: row.depth,
			context: !matching.has(row.task.id),
			hasChildren: (rows[index + 1]?.depth ?? 0) > row.depth,
		}))
		return {projectId, rows, count: rows.filter(row => !row.context).length}
	}).filter(project => project.count > 0)
}