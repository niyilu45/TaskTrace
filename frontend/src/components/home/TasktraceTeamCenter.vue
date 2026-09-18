<template>
	<div
		v-if="isLocalBuild"
		class="team-center"
	>
		<BaseButton
			class="team-import-button"
			aria-label="导入团队任务链接"
			title="导入团队任务链接"
			@click="showImport = true"
		>
			<Icon icon="users" />
			<span class="team-import-text">导入任务链接</span>
		</BaseButton>
		<BaseButton
			v-if="teamStore.conflictCount || teamStore.notificationCount"
			class="team-import-button"
			:aria-label="badgeLabel"
			:title="badgeLabel"
			@click="showActivity = true"
		>
			<Icon :icon="['far', 'bell']" />
			<span class="team-badge">{{ teamStore.conflictCount + teamStore.notificationCount }}</span>
		</BaseButton>

		<Modal
			:enabled="showImport"
			@close="closeImport"
			@submit="importTask"
		>
			<template #header>导入团队任务链接</template>
			<template #text>
				<div class="field">
					<label class="label" for="team-task-link">任务链接</label>
					<textarea
						id="team-task-link"
						v-model="link"
						class="textarea"
						rows="4"
						placeholder="粘贴以 tasktrace-team:// 开头的链接"
					/>
					<p class="help">链接只会导入共享任务及其子任务，不会导入对方的父任务或其他事项。</p>
				</div>
				<div class="field">
					<label class="label">保存到项目</label>
					<ProjectSearch
						:model-value="targetProject || undefined"
						@update:modelValue="targetProject = $event"
					/>
				</div>
				<div
					v-if="teamStore.status.repository && !teamStore.status.repository.shared"
					class="notification is-warning is-light"
				>
					本机 teamData 尚未检测到 Windows 共享。接收别人链接不受影响；若要分享自己的任务，请先在文件夹属性中共享 teamData。
				</div>
			</template>
		</Modal>

		<Modal
			:enabled="showActivity"
			@close="showActivity = false"
		>
			<template #header>团队协作通知与冲突</template>
			<template #text>
				<div v-if="teamStore.status.notifications?.length" class="team-notifications">
					<div class="team-notification-heading">
						<h3>成员更新</h3>
						<BaseButton @click="dismissNotifications">全部标为已读</BaseButton>
					</div>
					<p
						v-for="notice in teamStore.status.notifications"
						:key="notice.id"
					>
						<strong>{{ notice.actor }}</strong> 更新了“{{ notice.task_title }}”
						<span class="has-text-grey"> · {{ formatDisplayDate(notice.created) }}</span>
					</p>
				</div>
				<div v-if="teamStore.status.conflicts?.length" class="team-conflicts">
					<h3>需要处理的冲突</h3>
					<div
						v-for="conflict in teamStore.status.conflicts"
						:key="conflict.id"
						class="team-conflict"
					>
						<label class="label" :for="`team-conflict-${conflict.id}`">
							{{ conflict.task_title }} · {{ fieldLabel(conflict.field) }}
						</label>
						<select
							:id="`team-conflict-${conflict.id}`"
							v-model="resolutions[conflict.id || '']"
							class="input"
						>
							<option :value="conflict.base">保留上次同步值：{{ displayValue(conflict.field, conflict.base) }}</option>
							<option
								v-for="option in conflict.options"
								:key="`${option.author}-${option.value}`"
								:value="option.value"
							>
								采用 {{ option.author }} 的内容：{{ displayValue(conflict.field, option.value) }}
							</option>
						</select>
					</div>
					<XButton
						variant="primary"
						:loading="teamStore.loading"
						@click="resolveConflicts"
					>一次性应用所选结果</XButton>
				</div>
				<p v-if="!teamStore.status.notifications?.length && !teamStore.status.conflicts?.length">当前没有新的团队通知或冲突。</p>
			</template>
		</Modal>
	</div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, reactive, ref, watch} from 'vue'

import BaseButton from '@/components/base/BaseButton.vue'
import Modal from '@/components/misc/Modal.vue'
import ProjectSearch from '@/components/tasks/partials/ProjectSearch.vue'
import XButton from '@/components/input/Button.vue'
import type {IProject} from '@/modelTypes/IProject'
import {formatDisplayDate} from '@/helpers/time/formatDate'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {error, success} from '@/message'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

const teamStore = useTasktraceTeamStore()
const showImport = ref(false)
const showActivity = ref(false)
const link = ref('')
const targetProject = ref<IProject | null>(null)
const resolutions = reactive<Record<string, string>>({})
let timer: ReturnType<typeof setInterval> | null = null

const badgeLabel = computed(() => {
	const count = teamStore.conflictCount + teamStore.notificationCount
	return count ? `导入任务链接，另有 ${count} 条团队通知` : '导入团队任务链接'
})

watch(() => teamStore.status.conflicts, conflicts => {
	for (const conflict of conflicts ?? []) {
		if (conflict.id && !(conflict.id in resolutions)) resolutions[conflict.id] = conflict.base ?? ''
	}
}, {deep: true, immediate: true})

async function poll() {
	try {
		await teamStore.sync()
		if (teamStore.conflictCount) showActivity.value = true
	} catch {
		// An unavailable LAN share is reflected in each binding. Silent retry keeps offline work uninterrupted.
	}
}

onMounted(async () => {
	try { await teamStore.refresh() } catch { return }
	timer = setInterval(poll, 15_000)
	window.addEventListener('focus', poll)
})

onBeforeUnmount(() => {
	if (timer) clearInterval(timer)
	window.removeEventListener('focus', poll)
})

function closeImport() {
	showImport.value = false
	link.value = ''
	targetProject.value = null
}

async function importTask() {
	if (!link.value.trim() || !targetProject.value?.id) {
		error({message: '请填写任务链接并选择保存项目。'})
		return
	}
	try {
		await teamStore.importLink(link.value.trim(), targetProject.value.id)
		success({message: '团队任务已导入，后续修改会自动同步。'})
		closeImport()
	} catch (cause) {
		error(cause)
	}
}

function fieldLabel(field?: string) {
	if (field?.startsWith('outstanding:')) return `遗留事项 ${field.slice('outstanding:'.length)}`
	return ({title: '任务名', done: '完成状态', status: '任务状态', outstanding: '遗留事项'} as Record<string, string>)[field || ''] || field || '内容'
}

function displayValue(field?: string, value?: string) {
	if (field === 'done') return value === 'true' ? '已完成' : '未完成'
	if (field?.startsWith('outstanding:')) return value ? value.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim() || '含图片的遗留事项' : '删除此条'
	return value || '空'
}

async function resolveConflicts() {
	const conflicts = teamStore.status.conflicts ?? []
	try {
		for (const shareId of new Set(conflicts.map(item => item.share_id).filter(Boolean))) {
			await teamStore.resolve(shareId!, conflicts.filter(item => item.share_id === shareId).map(item => ({conflict_id: item.id, value: resolutions[item.id || ''] ?? item.base ?? ''})))
		}
		success({message: '冲突已处理并写回团队仓库。'})
		showActivity.value = false
	} catch (cause) {
		error(cause)
	}
}

async function dismissNotifications() {
	try { await teamStore.dismissNotifications() } catch (cause) { error(cause) }
}
</script>

<style scoped lang="scss">
.team-center { display: flex; align-items: stretch; }
.team-import-button { position: relative; padding-inline: .75rem; color: var(--grey-500); }
.team-import-text { margin-inline-start: .4rem; font-size: .85rem; font-weight: 600; }
.team-badge { position: absolute; inset-block-start: .35rem; inset-inline-end: .2rem; min-width: 1.1rem; padding: 0 .25rem; border-radius: 1rem; background: var(--danger); color: white; font-size: .65rem; text-align: center; }
.team-notifications, .team-conflicts { margin-block-end: 1.5rem; }
.team-notification-heading { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
.team-conflict { padding: .75rem; margin-block-end: .75rem; border: 1px solid var(--grey-200); border-radius: 8px; background: var(--grey-50); }
@media screen and (max-width: $tablet) { .team-import-text { display: none; } .team-import-button { padding-inline: .5rem; } }
</style>
