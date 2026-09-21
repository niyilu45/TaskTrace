<template>
	<div
		v-if="isLocalBuild"
		class="team-center"
		:class="{'team-center--page': !activityHost, 'team-center--activity-host': activityHost}"
	>
		<template v-if="!activityHost">
			<div class="team-page-actions">
				<XButton
					variant="secondary"
					icon="link"
					@click="showImport = true"
				>
					导入任务链接
				</XButton>
			</div>

			<section class="team-management-section">
				<div class="team-management-heading">
					<div>
						<h3>未分配人员</h3>
						<p class="has-text-grey">
							这些账户有 teamData 读写权限，但还不属于任何协作团队。
						</p>
					</div>
					<XButton
						variant="secondary"
						@click="addingMember = !addingMember"
					>
						{{ addingMember ? '取消添加' : '添加协作成员' }}
					</XButton>
				</div>
				<TeamMemberPicker
					v-if="addingMember"
					input-id="team-unassigned-member-search"
					:excluded="knownMembers"
					@select="grantMember"
				/>
				<div
					v-if="teamStore.status.unassigned_members?.length"
					class="team-unassigned-list"
				>
					<div
						v-for="member in teamStore.status.unassigned_members"
						:key="member"
						class="team-unassigned-row"
					>
						<span>{{ member }}</span>
						<XButton
							variant="secondary"
							:loading="teamStore.loading"
							@click="removeMember(member)"
						>
							删除权限
						</XButton>
					</div>
				</div>
				<p
					v-else
					class="team-empty-state"
				>
					当前没有未分配人员。
				</p>
			</section>

			<section class="team-management-section">
				<h3>协作任务团队</h3>
				<div
					v-for="binding in teamStore.status.bindings"
					:key="binding.share_id"
					class="team-binding-card"
				>
					<div>
						<strong>{{ binding.root_task_title || `任务 #${binding.root_task_id}` }}</strong>
						<p>{{ binding.members?.join('、') }}</p>
					</div>
					<XButton
						v-if="binding.member_link"
						variant="secondary"
						@click="copyTeamMembers(binding.member_link)"
					>
						分享成员
					</XButton>
				</div>
				<p
					v-if="!teamStore.status.bindings?.length"
					class="team-empty-state"
				>
					还没有协作任务团队。分享一个任务后会在这里显示。
				</p>
			</section>

			<section class="team-management-section">
				<label
					class="label"
					for="team-member-link"
				>导入团队成员链接</label>
				<textarea
					id="team-member-link"
					v-model="memberLink"
					class="textarea"
					rows="3"
					placeholder="粘贴以 tasktrace-team-members:// 开头的链接"
				/>
				<XButton
					variant="primary"
					:loading="teamStore.loading"
					@click="importTeamMembers"
				>
					导入成员并设置权限
				</XButton>
				<div
					v-if="memberImportResult"
					class="team-import-result"
				>
					<p v-if="memberImportResult.added?.length">
						已添加：{{ memberImportResult.added.join('、') }}
					</p>
					<p v-if="memberImportResult.skipped_self?.length">
						已跳过当前用户：{{ memberImportResult.skipped_self.join('、') }}
					</p>
					<p
						v-for="failure in memberImportResult.failed"
						:key="failure.username"
						class="has-text-danger"
					>
						{{ failure.username }} 导入失败：{{ failure.reason }}
					</p>
				</div>
			</section>
		</template>
		<Modal
			v-if="!activityHost"
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
			v-if="activityHost"
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
import {computed, onBeforeUnmount, onMounted, reactive, ref, watch} from 'vue'
import {useRouter} from 'vue-router'

import BaseButton from '@/components/base/BaseButton.vue'
import Modal from '@/components/misc/Modal.vue'
import ProjectSearch from '@/components/tasks/partials/ProjectSearch.vue'
import TeamMemberPicker from '@/components/tasks/partials/TeamMemberPicker.vue'
import XButton from '@/components/input/Button.vue'
import type {IProject} from '@/modelTypes/IProject'
import {formatDisplayDate} from '@/helpers/time/formatDate'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {error, getErrorText, success} from '@/message'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import type {TaskTraceTeamMemberCandidate, TaskTraceTeamMemberImportResult, TaskTraceTeamNotification} from '@/client/generated'

const props = withDefaults(defineProps<{
	activityHost?: boolean
}>(), {
	activityHost: false,
})

const activityHost = computed(() => props.activityHost)

const teamStore = useTasktraceTeamStore()
const router = useRouter()
const showImport = ref(false)
const showActivity = ref(false)
const link = ref('')
const repositoryOverride = ref('')
const targetProject = ref<IProject | null>(null)
const resolutions = reactive<Record<string, string>>({})
const addingMember = ref(false)
const memberLink = ref('')
const memberImportResult = ref<TaskTraceTeamMemberImportResult | null>(null)
let timer: ReturnType<typeof setInterval> | null = null

const knownMembers = computed(() => [
	teamStore.status.username || '',
	...(teamStore.status.repository?.candidates ?? []),
])

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
	try { await teamStore.refresh() } catch { /* Offline team repositories retry on the next poll. */ }
	if (activityHost.value) {
		window.addEventListener('focus', poll)
		window.addEventListener('tasktrace-team-activity-open', openActivity)
		timer = setInterval(poll, 15_000)
	}
})

onBeforeUnmount(() => {
	if (timer) clearInterval(timer)
	window.removeEventListener('focus', poll)
	window.removeEventListener('tasktrace-team-activity-open', openActivity)
})

function openActivity() {
	showActivity.value = true
}

async function grantMember(candidate: TaskTraceTeamMemberCandidate) {
	const accountName = candidate.account_name || candidate.username
	if (!accountName) return
	try {
		await teamStore.grantMember(accountName)
	} catch (cause) {
		if (!getErrorText(cause).includes('需要 Windows 管理员授权')) {
			error(cause)
			return
		}
		const confirmed = window.confirm(`为 ${accountName} 设置 teamData 共享读写权限需要 Windows 管理员授权。\n\n继续后 Windows 会显示用户账户控制窗口；本次操作完成后 TaskTrace 仍以普通权限运行。`)
		if (!confirmed) return
		try {
			await teamStore.grantMember(accountName, true)
		} catch (elevatedCause) {
			error(elevatedCause)
			return
		}
	}
	addingMember.value = false
	success({message: `已为 ${accountName} 设置 teamData 读写权限。`})
}

async function removeMember(member: string) {
	if (!window.confirm(`确定删除 ${member} 的 teamData 共享读写权限吗？`)) return
	try {
		await teamStore.removeMember(member)
		success({message: `已删除 ${member} 的 teamData 权限。`})
	} catch (cause) {
		error(cause)
	}
}

async function copyTeamMembers(link?: string) {
	if (!link) return
	await navigator.clipboard.writeText(link)
	success({message: '团队成员链接已复制。'})
}

async function importTeamMembers() {
	if (!memberLink.value.trim()) {
		error({message: '请先粘贴团队成员链接。'})
		return
	}
	try {
		memberImportResult.value = await teamStore.importMembers(memberLink.value.trim())
		const added = memberImportResult.value.added?.length ?? 0
		const failed = memberImportResult.value.failed?.length ?? 0
		if (added) success({message: `已成功导入 ${added} 位成员${failed ? `，另有 ${failed} 位导入失败` : ''}。`})
		else if (!failed) success({message: '链接中的成员已存在，当前用户已自动跳过。'})
	} catch (cause) {
		error(cause)
	}
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
	display: block;
}

.team-center--activity-host {
	display: contents;
}

.team-page-actions {
	display: flex;
	justify-content: flex-end;
	margin-block-end: .75rem;
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

.team-management-section {
	padding-block: 1.25rem;
	border-block-end: 1px solid var(--grey-200);
}

.team-management-section:first-of-type {
	padding-block-start: 0;
}

.team-management-section:last-child {
	padding-block-end: 0;
	border-block-end: 0;
}

.team-management-heading,
.team-unassigned-row,
.team-binding-card {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 1rem;
}

.team-management-heading p,
.team-binding-card p {
	margin: .15rem 0 0;
}

.team-unassigned-list {
	display: grid;
	gap: .5rem;
	margin-block-start: .75rem;
}

.team-unassigned-row,
.team-binding-card {
	padding: .7rem .8rem;
	border: 1px solid var(--grey-200);
	border-radius: 9px;
	background: var(--grey-50);
}

.team-binding-card {
	margin-block-start: .6rem;
}

.team-binding-card p,
.team-unassigned-row span {
	overflow-wrap: anywhere;
}

.team-empty-state {
	padding: .75rem;
	margin-block-start: .6rem;
	border-radius: 8px;
	background: var(--grey-50);
	color: var(--grey-500);
}

.team-import-result {
	padding: .65rem .75rem;
	margin-block-start: .75rem;
	border-radius: 8px;
	background: var(--grey-50);
}

.team-import-result p {
	margin: .15rem 0;
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
	.team-management-heading,
	.team-binding-card {
		align-items: flex-start;
		flex-direction: column;
	}
}
</style>
