import {reactive, watch, onBeforeUnmount} from 'vue'
const key = 'tasktrace-autosave'
export const autoSaveSettings = reactive({enabled: true, seconds: 30})
function load(value: string | null) {
	try {
		const parsed = JSON.parse(value || 'null')
		if (typeof parsed?.enabled === 'boolean') autoSaveSettings.enabled = parsed.enabled
		if (Number.isInteger(parsed?.seconds) && parsed.seconds >= 5 && parsed.seconds <= 3600) autoSaveSettings.seconds = parsed.seconds
	} catch { /* Invalid preferences use defaults. */ }
}
load(localStorage.getItem(key))
window.addEventListener('storage', event => { if (event.key === key) load(event.newValue) })
watch(autoSaveSettings, () => { try { localStorage.setItem(key, JSON.stringify(autoSaveSettings)) } catch { /* Apply for this session. */ } })
export function useAutoSave(check: () => Promise<void>) {
	let timer: ReturnType<typeof setInterval> | undefined
	const stop = watch(() => [autoSaveSettings.enabled, autoSaveSettings.seconds], () => {
		if (timer) clearInterval(timer)
		if (autoSaveSettings.enabled) timer = setInterval(() => { void check().catch(() => {}) }, autoSaveSettings.seconds * 1000)
	}, {immediate: true})
	onBeforeUnmount(() => { stop(); if (timer) clearInterval(timer) })
}
