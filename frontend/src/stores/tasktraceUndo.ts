import {computed, nextTick, ref} from 'vue'
import {defineStore} from 'pinia'
import {tasktraceUndoRead, tasktraceUndoCreate} from '@/client/generated'
import {undoBlockReason, undoInProgress, undoViewVersion, undoAffectedTaskIds} from '@/helpers/tasktraceUndo'
import {useProjectStore} from '@/stores/projects'

export const useTasktraceUndoStore = defineStore('tasktrace-undo', () => {
	const status = ref({id: 0, label: '', count: 0})
	const error = ref('')
	const message = ref('')
	let requestVersion = 0
	const canUndo = computed(() => !!status.value.id && !undoBlockReason.value)
	async function refresh() {
		if (undoInProgress.value) return
		const version = ++requestVersion
		const {data} = await tasktraceUndoRead()
		if (version === requestVersion) status.value = {id: data.id || 0, label: data.label || '', count: data.count || 0}
	}
	async function undo() {
		if (!canUndo.value) return
		undoInProgress.value = true
		++requestVersion
		error.value = ''
		message.value = ''
		try {
			const label = status.value.label
			const {data} = await tasktraceUndoCreate({body: {id: status.value.id}})
			status.value = {id: data.id || 0, label: data.label || '', count: data.count || 0}
			undoAffectedTaskIds.value = data.affected_task_ids || []
			undoViewVersion.value++
			await nextTick()
			try { await useProjectStore().loadAllProjects() } catch { error.value = '操作已撤销，项目列表刷新失败，请刷新页面。' }
			message.value = `已撤销：${label || '上一项操作'}。`
		} catch (cause) {
			const problem = cause as {detail?: string, message?: string}
			error.value = problem.detail || problem.message || '撤销失败，数据未覆盖，请刷新后重试。'
		} finally {
			await nextTick()
			undoInProgress.value = false
			try { await refresh() } catch { /* Keep the last known status and the visible error. */ }
		}
	}
	return {status, error, message, canUndo, refresh, undo}
})
