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
		<Modal
			:enabled="showImport"
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
						for="team-task-link"
					>任务链接</label>
					<textarea
						id="team-task-link"
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
						for="team-repository-override"
					>共享目录地址（可选）</label>
					<input
						id="team-repository-override"
						v-model="repositoryOverride"
						class="input"
						placeholder="\\\\10.143.58.8\\teamData"
					>
					<p class="help">
						正常情况下无需填写。如果链接里的电脑名无法访问，可填写资源管理器中能打开的完整 teamData 地址。
					</p>
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
			<template #header>
				团队协作通知与冲突
			</template>
			<template #text>
				<div class="team-profile">
					<img
						v-if="avatarFor(teamStore.status.username || '')"
						:src="avatarFor(teamStore.status.username || '')"
						alt=""
						class="team-avatar"
					>
					<span
						v-else
						class="team-avatar team-avatar--fallback"
					>{{ initials(teamStore.status.username || '') }}</span>
					<div>
						<strong>{{ teamStore.status.username }}</strong>
						<p class="has-text-grey">
							这是其他协作成员看到的身份。
						</p>
					</div>
					<BaseButton
						class="team-avatar-link"
						:to="{name: 'user.settings.avatar'}"
						@click="showActivity = false"
					>
						设置我的头像
					</BaseButton>
				</div>
				<div
					v-if="teamStore.status.notifications?.length"
					class="team-notifications"
				>
					<div class="team-notification-heading">
						<h3>成员更新</h3>
						<BaseButton @click="dismissNotifications">
							全部标为已读
						</BaseButton>
					</div>
					<div
						v-for="notice in teamStore.status.notifications"
						:key="notice.id"
						class="team-notification"
					>
						<img
							v-if="avatarFor(notice.actor || '', notice.avatar)"
							:src="avatarFor(notice.actor || '', notice.avatar)"
							alt=""
							class="team-avatar team-avatar--small"
						>
						<span
							v-else
							class="team-avatar team-avatar--small team-avatar--fallback"
						>{{ initials(notice.actor || '') }}</span>
						<div class="team-notification__content">
							<p>
								<strong>{{ notice.actor || '协作成员' }}</strong> 更新了事项
								<button
									type="button"
									class="team-notification__task-link"
									@click="openNotification(notice)"
								>
									“{{ notice.task_title || '团队任务' }}”
								</button>
							</p>
							<span class="has-text-grey">{{ formatDisplayDate(notice.created) }}</span>
						</div>
					</div>
				</div>
				<div
					v-if="teamStore.status.conflicts?.length"
					class="team-conflicts"
				>
					<h3>需要处理的冲突</h3>
					<div
						v-for="conflict in teamStore.status.conflicts"
						:key="conflict.id"
						class="team-conflict"
					>
						<label
							class="label"
							:for="`team-conflict-${conflict.id}`"
						>
							{{ conflict.task_title }} · {{ fieldLabel(conflict.field) }}
						</label>
						<select
							:id="`team-conflict-${conflict.id}`"
							v-model="resolutions[conflict.id || '']"
							class="input"
						>
							<option :value="conflict.base">
								保留上次同步值：{{ displayValue(conflict.field, conflict.base) }}
							</option>
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
					>
						一次性应用所选结果
					</XButton>
				</div>
				<p v-if="!teamStore.status.notifications?.length && !teamStore.status.conflicts?.length">
					当前没有新的团队通知或冲突。
				</p>
			</template>
		</Modal>
	</div>
</template>

<script setup lang="ts">
import {onBeforeUnmount, onMounted, reactive, ref, watch} from 'vue'
import {useRouter} from 'vue-router'

import BaseButton from '@/components/base/BaseButton.vue'
import Modal from '@/components/misc/Modal.vue'
import ProjectSearch from '@/components/tasks/partials/ProjectSearch.vue'
import XButton from '@/components/input/Button.vue'
import type {IProject} from '@/modelTypes/IProject'
import {formatDisplayDate} from '@/helpers/time/formatDate'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {error, success} from '@/message'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import type {TaskTraceTeamNotification} from '@/client/generated'

const teamStore = useTasktraceTeamStore()
const router = useRouter()
const showImport = ref(false)
const showActivity = ref(false)
const link = ref('')
const repositoryOverride = ref('')
const targetProject = ref<IProject | null>(null)
const resolutions = reactive<Record<string, string>>({})
let timer: ReturnType<typeof setInterval> | null = null

function avatarFor(username: string, preferred = '') {
	return preferred || teamStore.status.profiles?.find(profile => profile.username?.toLowerCase() === username.toLowerCase())?.avatar || ''
}

function initials(username: string) {
	return username.trim().slice(0, 2).toUpperCase() || '?'
}

watch(() => teamStore.status.conflicts, conflicts => {
	for (const conflict of conflicts ?? []) {
		if (conflict.id && !(conflict.id in resolutions)) resolutions[conflict.id] = conflict.base ?? ''
	}
}, {deep: true, immediate: true})

async function poll() {
	try {
		await teamStore.sync()
	} catch {
		// An unavailable LAN share is reflected in each binding. Silent retry keeps offline work uninterrupted.
	}
}

onMounted(async () => {
	window.addEventListener('focus', poll)
	window.addEventListener('tasktrace-team-activity-open', openActivity)
	try { await teamStore.refresh() } catch { /* Offline team repositories retry on the next poll. */ }
	timer = setInterval(poll, 15_000)
})

onBeforeUnmount(() => {
	if (timer) clearInterval(timer)
	window.removeEventListener('focus', poll)
	window.removeEventListener('tasktrace-team-activity-open', openActivity)
})

function openActivity() {
	showActivity.value = true
}

function closeImport() {
	showImport.value = false
	link.value = ''
	repositoryOverride.value = ''
	targetProject.value = null
}

async function importTask() {
	if (!link.value.trim() || !targetProject.value?.id) {
		error({message: '请填写任务链接并选择保存项目。'})
		return
	}
	try {
		await teamStore.importLink(link.value.trim(), targetProject.value.id, repositoryOverride.value)
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

async function openNotification(notice: TaskTraceTeamNotification) {
	const taskId = Number(notice.task_id || 0)
	if (!taskId) return
	showActivity.value = false
	const commentId = Number(notice.comment_id || 0)
	await router.push({
		name: 'task.detail',
		params: {id: taskId},
		...(commentId ? {hash: `#comment-${commentId}`} : {}),
	})
	if (notice.id) {
		try { await teamStore.dismissNotifications([notice.id]) } catch (cause) { error(cause) }
	}
}
</script>

<style scoped lang="scss">
.team-center {
	display: flex;
	align-items: stretch;
}

.team-import-button {
	position: relative;
	padding-inline: .75rem;
	color: var(--grey-500);
}

.team-import-text {
	margin-inline-start: .4rem;
	font-size: .85rem;
	font-weight: 600;
}

.team-profile {
	display: flex;
	align-items: center;
	gap: .75rem;
	padding: .75rem;
	margin-block-end: 1rem;
	border: 1px solid var(--grey-200);
	border-radius: 10px;
	background: var(--grey-50);
}

.team-profile p {
	margin: .1rem 0 0;
	font-size: .8rem;
}

.team-avatar-link {
	margin-inline-start: auto;
	color: var(--primary);
	font-weight: 600;
}

.team-avatar {
	inline-size: 40px;
	block-size: 40px;
	flex: 0 0 40px;
	border-radius: 50%;
	object-fit: cover;
}

.team-avatar--small {
	inline-size: 32px;
	block-size: 32px;
	flex-basis: 32px;
}

.team-avatar--fallback {
	display: grid;
	place-items: center;
	background: var(--primary);
	color: var(--white);
	font-weight: 700;
	font-size: .75rem;
}

.team-notifications,
.team-conflicts {
	margin-block-end: 1.5rem;
}

.team-notification-heading {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 1rem;
}

.team-notification {
	display: flex;
	align-items: flex-start;
	gap: .75rem;
	padding: .75rem;
	margin-block-end: .5rem;
	border: 1px solid var(--grey-200);
	border-radius: 10px;
	background: var(--white);
	color: var(--text);
	box-shadow: var(--shadow-xs);
}

.team-notification__content {
	min-inline-size: 0;
}

.team-notification__content p {
	margin: 0 0 .2rem;
	color: var(--text);
	overflow-wrap: anywhere;
}

.team-notification__content strong {
	color: var(--text-strong);
}

.team-notification__task-link {
	padding: 0;
	border: 0;
	background: transparent;
	color: var(--primary);
	font: inherit;
	font-weight: 700;
	text-align: start;
	text-decoration: underline;
	text-underline-offset: 2px;
	cursor: pointer;
}

.team-notification__task-link:hover,
.team-notification__task-link:focus-visible {
	color: var(--link-hover);
}

.team-notification__content span {
	font-size: .8rem;
}

.team-conflict {
	padding: .75rem;
	margin-block-end: .75rem;
	border: 1px solid var(--grey-200);
	border-radius: 8px;
	background: var(--grey-50);
}

@media screen and (max-width: $tablet) {
	.team-import-text {
		display: none;
	}

	.team-import-button {
		padding-inline: .5rem;
	}
}
</style>
