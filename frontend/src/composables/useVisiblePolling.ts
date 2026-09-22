import {onMounted, onBeforeUnmount} from 'vue'

// One request at a time, and no background timers for hidden tabs. Restoring
// visibility catches up immediately without focus/visibility event duplicates.
export function useVisiblePolling(read: () => Promise<unknown>, interval: number, options: {
	immediate?: boolean
	enabled?: () => boolean
	catchUpOnResume?: boolean
	onError?: (error: unknown) => void
} = {}) {
	let mounted = false
	let timer: ReturnType<typeof setTimeout> | undefined
	let active: Promise<void> | undefined
	let catchUpPending = false
	let lastStarted = -Infinity
	const visible = () => document.visibilityState === 'visible'
	let wasVisible = visible()
	function clear() { if (timer !== undefined) clearTimeout(timer); timer = undefined }
	function schedule() {
		clear()
		if (mounted && visible()) timer = setTimeout(() => { void refresh() }, interval)
	}
	function refresh(catchUp = false): Promise<void> {
		if (active) { if (catchUp) catchUpPending = true; return active }
		if (!mounted || !visible()) return Promise.resolve()
		if (!catchUp && options.enabled && !options.enabled()) { schedule(); return Promise.resolve() }
		clear()
		catchUpPending = false
		lastStarted = Date.now()
		const request = Promise.resolve().then(read).then(() => undefined, error => { options.onError?.(error) }).finally(() => {
			if (active === request) active = undefined
			if (catchUpPending && mounted && visible()) void refresh(true)
			else schedule()
		})
		active = request
		return request
	}
	function resume() {
		if (!visible()) { wasVisible = false; clear(); return }
		const restored = !wasVisible
		wasVisible = true
		if (!restored && Date.now() - lastStarted < 1000) { if (!active) schedule(); return }
		void refresh(options.catchUpOnResume)
	}
	onMounted(() => {
		mounted = true
		document.addEventListener('visibilitychange', resume)
		window.addEventListener('focus', resume)
		if (options.immediate !== false) void refresh()
		else schedule()
	})
	onBeforeUnmount(() => {
		mounted = false
		catchUpPending = false
		clear()
		document.removeEventListener('visibilitychange', resume)
		window.removeEventListener('focus', resume)
	})
	return {refresh}
}
