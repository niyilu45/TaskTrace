import {ref, watch} from 'vue'
import {allTaskStatuses, legacyTaskStatusSelection} from '@/helpers/taskStatusFilter'
import type {TaskStatus} from '@/types/ITaskStatus'

// Keep the previous scope on first use; an explicitly empty selection stays empty.
export function useTaskStatusFilter(key: string, legacy: () => string) {
	let initial = legacyTaskStatusSelection('pending')
	try {
		const saved = JSON.parse(localStorage.getItem(key) || 'null')
		initial = Array.isArray(saved)
			? allTaskStatuses.filter(state => saved.includes(state))
			: legacyTaskStatusSelection(legacy())
	} catch { /* Keep usable defaults if browser storage is unavailable. */ }
	const selected = ref<TaskStatus[]>(initial)
	watch(selected, value => {
		try { localStorage.setItem(key, JSON.stringify(value)) } catch { /* Selection still applies. */ }
	})
	return selected
}
