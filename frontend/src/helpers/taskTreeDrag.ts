import type {ITask} from '@/modelTypes/ITask'

export type TaskDropZone = 'before' | 'inside' | 'after'

export interface TaskMovePlan {
	parentId: number
	beforeTaskId: number
}

function taskParentMap(tasks: ITask[]): Map<number, number> {
	const visibleIds = new Set(tasks.map(item => item.id))
	return new Map(tasks.map(task => [
		task.id,
		(task.relatedTasks?.parenttask ?? [])
			.map(parent => parent.id)
			.filter(id => visibleIds.has(id))
			.sort((left, right) => left - right)[0] ?? 0,
	]))
}

export function taskParentId(task: ITask, tasks: ITask[]): number {
	return taskParentMap(tasks).get(task.id) ?? 0
}

function subtreeIds(taskId: number, tasks: ITask[], parents: Map<number, number>): Set<number> {
	const result = new Set<number>([taskId])
	const queue = [taskId]
	while (queue.length) {
		const parentId = queue.shift()!
		for (const task of tasks) {
			if (!result.has(task.id) && parents.get(task.id) === parentId) {
				result.add(task.id)
				queue.push(task.id)
			}
		}
	}
	return result
}

export function taskSubtreeIds(taskId: number, tasks: ITask[]): Set<number> {
	return subtreeIds(taskId, tasks, taskParentMap(tasks))
}

export function taskMovePlan(
	tasks: ITask[],
	draggedTaskId: number,
	targetTaskId: number,
	zone: TaskDropZone,
): TaskMovePlan | null {
	if (draggedTaskId === targetTaskId) return null
	const target = tasks.find(task => task.id === targetTaskId)
	const parents = taskParentMap(tasks)
	if (!target || subtreeIds(draggedTaskId, tasks, parents).has(targetTaskId)) return null

	if (zone === 'inside') {
		return {parentId: targetTaskId, beforeTaskId: 0}
	}

	const parentId = parents.get(target.id) ?? 0
	if (zone === 'before') {
		return {parentId, beforeTaskId: targetTaskId}
	}

	const siblings = tasks
		.filter(task => task.id !== draggedTaskId && parents.get(task.id) === parentId)
	const targetIndex = siblings.findIndex(task => task.id === targetTaskId)
	return {
		parentId,
		beforeTaskId: targetIndex >= 0 ? siblings[targetIndex + 1]?.id ?? 0 : 0,
	}
}
