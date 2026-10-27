import type {ITask} from '@/modelTypes/ITask'
import {invalidateCachedTasks} from '@/helpers/taskCache'

export interface TasktraceTaskChange {
	task_id: number
	project_id: number
	related_task_id?: number
	deleted?: boolean
}

export const TASKTRACE_LOCAL_TASK_CHANGES = 'tasktrace-local-task-changes'

// Use the same committed-change queue as server events, so open cards update
// without depending on the WebSocket round trip or reloading dirty editors.
export function publishTasktraceTaskChanges(changes: TasktraceTaskChange[]) {
	invalidateCachedTasks(changes.flatMap(change => [change.task_id, change.related_task_id ?? 0]).filter(id => id > 0))
	if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(TASKTRACE_LOCAL_TASK_CHANGES, {detail: changes}))
}

export function tasktraceDeletionScope(task: ITask, known: ITask[], cascade = true) {
	const byId = new Map(known.map(item => [item.id, item]))
	if (!byId.has(task.id)) byId.set(task.id, task)
	const ids = new Set<number>()
	const pending = [task.id]
	while (pending.length) {
		const id = pending.pop()!
		if (id <= 0 || ids.has(id)) continue
		ids.add(id)
		if (!cascade) continue
		const current = byId.get(id)
		for (const child of current?.relatedTasks?.subtask ?? []) {
			if (!byId.has(child.id)) byId.set(child.id, child)
			pending.push(child.id)
		}
		for (const candidate of byId.values()) {
			if (candidate.parentTaskId === id || candidate.relatedTasks?.parenttask?.some(parent => parent.id === id)) pending.push(candidate.id)
		}
	}
	const parents = new Set<number>()
	for (const id of ids) {
		const item = byId.get(id)
		if (item?.parentTaskId && !ids.has(item.parentTaskId)) parents.add(item.parentTaskId)
		for (const parent of item?.relatedTasks?.parenttask ?? []) if (!ids.has(parent.id)) parents.add(parent.id)
	}
	for (const candidate of byId.values()) {
		if (!ids.has(candidate.id) && candidate.relatedTasks?.subtask?.some(child => ids.has(child.id))) parents.add(candidate.id)
	}
	return {ids, parents, tasks: byId}
}
