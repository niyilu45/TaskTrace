import {onScopeDispose, watch} from 'vue'
import {useWebSocket} from './useWebSocket'

export interface TasktraceTaskChange {
	task_id: number
	project_id: number
	related_task_id?: number
}

// Idle views perform no reads. Changes during a read or edit remain pending.
export function useTasktraceTaskChanges(
	refresh: () => Promise<unknown>,
	matches: (change: TasktraceTaskChange) => boolean,
	blocked: () => boolean = () => false,
) {
	const {subscribe, authenticated} = useWebSocket()
	let pending = false
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
		pending = false
		running = true
		let failed = false
		try { if (await refresh() === false) failed = true } catch { failed = true }
		finally { running = false; pending ||= failed; schedule(failed ? 3000 : 350) }
	}
	const unsubscribe = subscribe('tasktrace.task.changed', message => {
		const change = message.data as TasktraceTaskChange | undefined
		if (!change || !matches(change)) return
		pending = true
		schedule()
	})
	watch(blocked, () => schedule())
	watch(authenticated, value => {
		if (value && wasAuthenticated) { pending = true; schedule() }
		if (value) wasAuthenticated = true
	})
	const visible = () => schedule()
	document.addEventListener('visibilitychange', visible)
	onScopeDispose(() => {
		disposed = true
		if (timer) clearTimeout(timer)
		unsubscribe()
		document.removeEventListener('visibilitychange', visible)
	})
}
