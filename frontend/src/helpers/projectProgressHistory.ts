import {reactive, ref, type InjectionKey} from 'vue'
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

// Share reads within a display view. Live updates invalidate only affected tasks;
// a manual refresh/reconnect clears all histories, including same-count edits.
export function createProjectProgressHistory() {
	const reads = new Map<number, Promise<TaskComment[]>>()
	const generation = ref(0)
	const revisions = reactive(new Map<number, number>())
	const revisionFor = (taskId: number) => `${generation.value}:${revisions.get(taskId) || 0}`
	function invalidate(taskIds: Iterable<number>) {
		for (const id of new Set(taskIds)) {
			reads.delete(id)
			revisions.set(id, (revisions.get(id) || 0) + 1)
		}
	}
	function read(taskId: number): Promise<TaskComment[]> {
		const existing = reads.get(taskId)
		if (existing) return existing
		const version = revisionFor(taskId)
		const ensureCurrent = () => {
			if (version !== revisionFor(taskId)) throw new ProgressReadCancelled()
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
	return {read, revisionFor, invalidate, clear: () => { generation.value++; revisions.clear(); reads.clear() }}
}

export const projectProgressHistoryKey: InjectionKey<ReturnType<typeof createProjectProgressHistory>> = Symbol('project-progress-history')
