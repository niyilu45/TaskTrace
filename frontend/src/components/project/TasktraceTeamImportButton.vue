<template>
	<XButton
		variant="secondary"
		icon="link"
		@click="showImport = true"
	>
		导入共享任务链接
	</XButton>
	<Modal
		:enabled="showImport"
		submit-label="导入任务"
		@close="closeImport"
		@submit="importTask"
	>
		<template #header>
			导入团队任务链接
		</template>
		<template #text>
			<div class="field">
				<label
					class="label"
					:for="`project-team-task-link-${projectId}`"
				>任务链接</label>
				<textarea
					:id="`project-team-task-link-${projectId}`"
					v-model="link"
					class="textarea"
					rows="4"
					placeholder="粘贴以 tasktrace-team:// 开头的链接"
				/>
				<p class="help">
					链接只会导入共享任务及其子任务，不会导入对方的父任务或其他事项。
				</p>
			</div>
			<div class="field">
				<label
					class="label"
					:for="`project-team-repository-${projectId}`"
				>共享目录地址（可选）</label>
				<input
					:id="`project-team-repository-${projectId}`"
					v-model="repositoryOverride"
					class="input"
					placeholder="\\\\10.143.58.8\\teamData"
				>
				<p class="help">
					链接中的电脑名无法访问时，可填写资源管理器中能打开的完整 teamData 地址。
				</p>
			</div>
			<p>保存到当前项目：<strong>{{ projectName || `项目 #${projectId}` }}</strong></p>
			<div
				v-if="teamStore.status.repository && !teamStore.status.repository.shared"
				class="notification is-warning is-light"
			>
				本机 teamData 尚未检测到 Windows 共享。接收别人链接不受影响；若要分享自己的任务，请先在文件夹属性中共享 teamData。
			</div>
		</template>
	</Modal>
</template>

<script setup lang="ts">
import {ref} from 'vue'

import XButton from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import {error, success} from '@/message'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

const props = defineProps<{projectId: number, projectName?: string}>()
const emit = defineEmits<{imported: []}>()
const teamStore = useTasktraceTeamStore()
const showImport = ref(false)
const link = ref('')
const repositoryOverride = ref('')

function closeImport() {
	if (teamStore.loading) return
	showImport.value = false
	link.value = ''
	repositoryOverride.value = ''
}

async function importTask() {
	if (!link.value.trim()) {
		error({message: '请先粘贴共享任务链接。'})
		return
	}
	try {
		await teamStore.importLink(link.value.trim(), props.projectId, repositoryOverride.value)
		success({message: '团队任务已导入到当前项目，后续修改会自动同步。'})
		showImport.value = false
		link.value = ''
		repositoryOverride.value = ''
		emit('imported')
	} catch (cause) {
		error(cause)
	}
}
</script>
