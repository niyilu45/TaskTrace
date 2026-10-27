import {onScopeDispose, watch} from 'vue'
import {useWebSocket} from './useWebSocket'

import {TASKTRACE_LOCAL_TASK_CHANGES, type TasktraceTaskChange} from '@/helpers/tasktraceTaskEvents'
export type {TasktraceTaskChange} from '@/helpers/tasktraceTaskEvents'

// Idle views perform no reads. Changes during a read or edit remain pending.
export function useTasktraceTaskChanges(
	refresh: (changes: TasktraceTaskChange[] | null) => Promise<unknown>,
	matches: (change: TasktraceTaskChange) => boolean,
	blocked: () => boolean = () => false,
) {
	const {subscribe, authenticated} = useWebSocket()
	let pending = false
	let fullRefresh = false
	const changes = new Map<string, TasktraceTaskChange>()
	let running = false
	let disposed = false
	let timer: ReturnType<typeof setTimeout> | undefined
	let wasAuthenticated = authenticated.value
	function schedule(delay = 350) {
		if (disposed || !pending || running || timer || blocked() || document.hidden) return
		timer = setTimeout(() => { timer = undefined; void flush() }, delay)
	}
	async function flush() {
		if (disposed || blocked() || document.hidden) return
		const batch = fullRefresh ? null : [...changes.values()]
		pending = false
		fullRefresh = false
		changes.clear()
		running = true
		let failed = false
		try { if (await refresh(batch) === false) failed = true } catch { failed = true }
		finally {
			running = false
			if (failed) {
				pending = true
				if (batch === null) fullRefresh = true
				else for (const change of batch) changes.set(`${change.task_id}:${change.related_task_id || 0}`, change)
			}
			schedule(failed ? 3000 : 350)
		}
	}
	function enqueue(change?: TasktraceTaskChange) {
		if (!change || !matches(change)) return
		changes.set(`${change.task_id}:${change.related_task_id || 0}`, change)
		pending = true
		schedule()
	}
	const unsubscribe = subscribe('tasktrace.task.changed', message => enqueue(message.data as TasktraceTaskChange | undefined))
	const localChanges = (event: Event) => {
		const detail = (event as CustomEvent<TasktraceTaskChange[]>).detail
		if (Array.isArray(detail)) for (const change of detail) enqueue(change)
	}
	window.addEventListener(TASKTRACE_LOCAL_TASK_CHANGES, localChanges)
	watch(blocked, () => schedule())
	watch(authenticated, value => {
		if (value && wasAuthenticated) { pending = true; fullRefresh = true; schedule() }
		if (value) wasAuthenticated = true
	})
	const visible = () => schedule()
	document.addEventListener('visibilitychange', visible)
	onScopeDispose(() => {
		disposed = true
		if (timer) clearTimeout(timer)
		unsubscribe()
		window.removeEventListener(TASKTRACE_LOCAL_TASK_CHANGES, localChanges)
		document.removeEventListener('visibilitychange', visible)
	})
}
