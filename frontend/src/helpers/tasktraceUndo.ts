import {computed, onBeforeUnmount, ref, shallowReactive, watch, type WatchSource} from 'vue'
import {isLocalBuild} from '@/helpers/tasktraceLocal'

export const undoInProgress = ref(false)
export const undoViewVersion = ref(0)
export const undoAffectedTaskIds = ref<number[]>([])
export const undoPendingWrites = ref(0)
const guards = shallowReactive(new Map<symbol, string>())
export const undoBlockReason = computed(() => undoInProgress.value ? '正在撤销…' : undoPendingWrites.value ? '正在保存，请稍候。' : guards.values().next().value || '')
let refreshHandler: (() => Promise<void>) | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined
const pending = new WeakMap<object, () => void>()

export function useTasktraceUndoGuard(dirty: WatchSource<boolean>, reason: string) {
	const key = Symbol('undo-guard')
	const stop = watch(dirty, value => {
		if (isLocalBuild && value) guards.set(key, reason)
		else guards.delete(key)
	}, {immediate: true, flush: 'sync'})
	onBeforeUnmount(() => { stop(); guards.delete(key) })
}

export function setUndoRefreshHandler(handler?: () => Promise<void>) {
	refreshHandler = handler
}

export function refreshUndoSoon() {
	if (!isLocalBuild || !refreshHandler) return
	if (refreshTimer) clearTimeout(refreshTimer)
	refreshTimer = setTimeout(() => { void refreshHandler?.().catch(() => {}) }, 80)
}

export function isUndoMutation(method: string | undefined, address: string) {
	if (!isLocalBuild || !method || ['GET', 'HEAD', 'OPTIONS'].includes(method.toUpperCase())) return false
	const path = new URL(address, window.location.origin).pathname.replace(/^\/api\/v[12]/, '')
	return /^\/tasks(?:\/|$)/.test(path) && !/\/(attachments|reactions|read|mark-read|subscriptions|time-entries)(?:\/|$)/.test(path)
		|| /^\/projects\/\d+\/tasks(?:\/|$)/.test(path)
}

export function undoGroupHeaders(group?: string): Record<string, string> {
	if (!isLocalBuild) return {}
	return {'X-TaskTrace-Undo': '1', 'X-TaskTrace-Undo-Group': group || crypto.randomUUID()}
}

export function isUndoSafeRoute(name: unknown) {
	return ['home', 'task.detail', 'project.index', 'project.view', 'projects.index', 'tasks.range'].includes(String(name))
}

export function beginUndoWrite(key: object) {
	if (pending.has(key)) return
	undoPendingWrites.value++
	pending.set(key, () => { undoPendingWrites.value = Math.max(0, undoPendingWrites.value - 1) })
}

export function finishUndoWrite(key?: object) {
	if (!key) return
	const finish = pending.get(key)
	if (!finish) return
	pending.delete(key)
	finish()
	refreshUndoSoon()
}

export function isUndoTextTarget(target: EventTarget | null) {
	if (!(target instanceof Element)) return false
	return !!target.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"]), .ProseMirror')
}
