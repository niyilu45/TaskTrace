import {tasksRead, type Task} from '@/client/generated'
export const MAX_TASK_DEPTH = 5
export const TASK_DEPTH_MESSAGE = '任务最多支持 5 级（顶层任务为第 1 级，项目不计入），无法继续添加子任务。'
export async function taskHierarchySpan(taskId: number, kind: 'parenttask' | 'subtask' = 'parenttask') {
	const cache = new Map<number, Promise<Task>>()
	async function walk(id: number, level: number): Promise<number> {
		if (level >= MAX_TASK_DEPTH) return 1
		if (!cache.has(id)) cache.set(id, tasksRead({path: {task: id}}).then(result => result.data))
		const task = await cache.get(id)!
		let span = 1
		for (const related of task.related_tasks?.[kind] || []) {
			if (!related.id) continue
			span = Math.max(span, 1 + await walk(related.id, level + 1))
			if (span >= MAX_TASK_DEPTH - level + 1) break
		}
		return span
	}
	return walk(taskId, 1)
}
export async function assertCanAddSubtask(taskId: number) {
	if (await taskHierarchySpan(taskId) >= MAX_TASK_DEPTH) throw new Error(TASK_DEPTH_MESSAGE)
}