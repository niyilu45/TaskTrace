import type {InjectionKey} from 'vue'
import {taskCommentsList, type TaskComment} from '@/client/generated'
import {queueProgressRead} from '@/helpers/projectProgress'

class ProgressReadCancelled extends Error {
	constructor() {
		super('This display refresh was superseded')
		this.name = 'AbortError'
	}
}

export function isProgressReadCancelled(error: unknown) {
	return error instanceof ProgressReadCancelled
}

// One reader belongs to one display view. A refresh clears it, so comment edits
// are re-read even when the task timestamp and comment count did not change.
export function createProjectProgressHistory() {
	const reads = new Map<number, Promise<TaskComment[]>>()
	let generation = 0
	function read(taskId: number): Promise<TaskComment[]> {
		const existing = reads.get(taskId)
		if (existing) return existing
		const version = generation
		const ensureCurrent = () => {
			if (version !== generation) throw new ProgressReadCancelled()
		}
		const request = (async () => {
			const history: TaskComment[] = []
			for (let page = 1; ; page++) {
				const result = await queueProgressRead(() => {
					ensureCurrent()
					return taskCommentsList({path: {task: taskId}, query: {page, per_page: 100, order_by: 'desc'}})
				})
				ensureCurrent()
				const items = result.data.items || []
				history.push(...items)
				if (page >= (result.data.total_pages || 1) || !items.length) return history
			}
		})()
		reads.set(taskId, request)
		void request.catch(() => {
			if (reads.get(taskId) === request) reads.delete(taskId)
		})
		return request
	}
	return {read, clear: () => { generation++; reads.clear() }}
}

export const projectProgressHistoryKey: InjectionKey<ReturnType<typeof createProjectProgressHistory>> = Symbol('project-progress-history')
