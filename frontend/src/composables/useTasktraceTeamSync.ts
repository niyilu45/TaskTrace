import {ref} from 'vue'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import {getErrorText} from '@/message'

// Synchronize saved collaboration data without re-mounting the project or its editors.
export function useTasktraceTeamSync() {
	const teamStore = useTasktraceTeamStore()
	const syncing = ref(false)
	const syncMessage = ref('')
	const syncDetails = ref<string[]>([])

	async function syncNow() {
		if (syncing.value) return
		syncing.value = true
		syncMessage.value = '正在与团队共享目录同步…'
		syncDetails.value = []
		try {
			const result = await teamStore.sync()
			if (result.enabled !== true) {
				syncMessage.value = '协作同步不可用，请检查本机协作设置。'
				return
			}
			const bindings = result.bindings ?? []
			const failed = bindings.filter(binding => binding.last_error?.trim())
			const conflicts = bindings.reduce((count, binding) => count + (binding.conflicts?.length ?? 0), 0)
			syncDetails.value = failed.map(binding => `${binding.root_task_title || `任务 #${binding.root_task_id}`}：${binding.last_error}`)
			if (!bindings.length) {
				syncMessage.value = '当前没有需要同步的协作任务。'
			} else if (failed.length) {
				syncMessage.value = `同步未全部完成：${failed.length} / ${bindings.length} 项协作任务同步失败，请查看下方原因后重试。`
			} else {
				syncMessage.value = `本机与共享目录同步完成，共 ${bindings.length} 项协作任务。`
			}
			if (conflicts) syncDetails.value.push(`有 ${conflicts} 项冲突待处理，请打开通知查看差异。`)
		} catch (cause) {
			syncMessage.value = `同步未完成：${getErrorText(cause)}`
		} finally {
			syncing.value = false
		}
	}

	return {syncing, syncMessage, syncDetails, syncNow}
}
