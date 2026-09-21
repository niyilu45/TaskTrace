import type {ProgressTask} from '@/helpers/projectProgress'

const CACHE_LIMIT = 8
const taskCache = new Map<number, ProgressTask[]>()

export function readProjectProgressCache(projectId: number) {
	const tasks = taskCache.get(projectId)
	if (!tasks) return []
	taskCache.delete(projectId)
	taskCache.set(projectId, tasks)
	return tasks
}

export function writeProjectProgressCache(projectId: number, tasks: ProgressTask[]) {
	taskCache.delete(projectId)
	taskCache.set(projectId, tasks)
	while (taskCache.size > CACHE_LIMIT) taskCache.delete(taskCache.keys().next().value!)
}

export function clearProjectProgressCache() {
	taskCache.clear()
}
