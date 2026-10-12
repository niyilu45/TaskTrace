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
						<h3>所有协作人员</h3>
						<p class="has-text-grey">
							这里设置 teamData 的最高权限；具体任务和遗留事项仍可单独设为只读或无权限。
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
					v-if="allMembers.length"
					class="team-unassigned-list"
				>
					<div
						v-for="member in allMembers"
						:key="teamStore.memberKey(member.username)"
						class="team-unassigned-row"
					>
						<div class="team-member-summary">
							<TeamMemberIdentity :username="member.username" />
							<span
								v-if="member.isCurrent"
								class="tag is-primary is-light"
							>本机用户</span>
						</div>
						<div class="team-member-actions">
							<label :for="`team-access-${teamStore.memberKey(member.username)}`">最高权限</label>
							<select
								:id="`team-access-${teamStore.memberKey(member.username)}`"
								class="select"
								:value="member.access"
								:disabled="member.isCurrent || changingMember === member.username"
								@change="changeMemberAccess(member.username, ($event.target as HTMLSelectElement).value as 'read' | 'write')"
							>
								<option value="read">
									仅读
								</option>
								<option value="write">
									读写
								</option>
							</select>
							<XButton
								v-if="member.unassigned"
								variant="secondary"
								:loading="removingMember === member.username"
								:disabled="!!removingMember || !!changingMember"
								@click="requestRemoveMember(member.username)"
							>
								删除权限
							</XButton>
						</div>
					</div>
				</div>
				<p
					v-else
					class="team-empty-state"
				>
					当前没有协作人员。
				</p>
				<p
					v-if="memberActionError"
					class="team-member-error has-text-danger"
					role="alert"
				>
					{{ memberActionError }}
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
						<p class="team-member-names">
							<TeamMemberIdentity
								v-for="member in teamStore.uniqueMembers(binding.members ?? [])"
								:key="teamStore.memberKey(member)"
								:username="member"
							/>
						</p>
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
			v-if="pendingRemoval"
			:enabled="true"
			submit-label="删除权限"
			@close="cancelRemoveMember"
			@submit="confirmRemoveMember"
		>
			<template #header>
				删除协作人员权限
			</template>
			<template #text>
				<p>确定删除 <strong>{{ pendingRemoval }}</strong> 的 teamData 共享读写权限吗？</p>
				<p class="has-text-grey">
					如果 Windows 要求管理员权限，确认后会弹出一次用户账户控制窗口。
				</p>
				<p
					v-if="removingMember"
					role="status"
				>
					正在删除权限…
				</p>
			</template>
		</Modal>
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

		<TeamConflictResolver
			v-if="activityHost && showActivity"
			@close="showActivity = false"
		/>
	</div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref} from 'vue'
import {useVisiblePolling} from '@/composables/useVisiblePolling'

import TeamConflictResolver from './TeamConflictResolver.vue'
import Modal from '@/components/misc/Modal.vue'
import ProjectSearch from '@/components/tasks/partials/ProjectSearch.vue'
import TeamMemberPicker from '@/components/tasks/partials/TeamMemberPicker.vue'
import TeamMemberIdentity from '@/components/tasks/partials/TeamMemberIdentity.vue'
import XButton from '@/components/input/Button.vue'
import type {IProject} from '@/modelTypes/IProject'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {error, getErrorText, success} from '@/message'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import type {TaskTraceTeamMemberCandidate, TaskTraceTeamMemberImportResult} from '@/client/generated'

const props = withDefaults(defineProps<{
	activityHost?: boolean
}>(), {
	activityHost: false,
})

const activityHost = computed(() => props.activityHost)

const teamStore = useTasktraceTeamStore()
const showImport = ref(false)
const showActivity = ref(false)
const link = ref('')
const repositoryOverride = ref('')
const targetProject = ref<IProject | null>(null)
const addingMember = ref(false)
const memberLink = ref('')
const memberImportResult = ref<TaskTraceTeamMemberImportResult | null>(null)
const pendingRemoval = ref('')
const removingMember = ref('')
const changingMember = ref('')
const memberActionError = ref('')
useVisiblePolling(poll, 15_000, {enabled: () => activityHost.value})

const knownMembers = computed(() => [
	teamStore.status.username || '',
	...teamStore.memberRoster,
])

const allMembers = computed(() => teamStore.memberRoster.map(username => ({
	username,
	access: teamStore.maximumAccessFor(username) ?? 'read',
	isCurrent: teamStore.memberKey(username) === teamStore.memberKey(teamStore.status.username || ''),
	unassigned: (teamStore.status.unassigned_members ?? []).some(member => teamStore.memberKey(member) === teamStore.memberKey(username)),
})))

async function poll() {
	try {
		// The desktop process owns LAN synchronization. Browser tabs only read its result.
		await teamStore.refresh()
	} catch {
		// An unavailable LAN share is reflected in each binding. Silent retry keeps offline work uninterrupted.
	}
}

onMounted(() => {
	if (!activityHost.value) void teamStore.refresh().catch(() => undefined)
	if (activityHost.value) {
		window.addEventListener('tasktrace-team-activity-open', openActivity)
	}
})

onBeforeUnmount(() => {
	window.removeEventListener('tasktrace-team-activity-open', openActivity)
})

function openActivity() {
	showActivity.value = true
}

function needsAdministratorAuthorization(cause: unknown) {
	const value = cause as {status?: number, response?: {status?: number}, reason?: {response?: {status?: number}}}
	const status = value?.reason?.response?.status ?? value?.response?.status ?? value?.status
	const message = getErrorText(cause).toLowerCase()
	return status === 403 || [
		'windows administrator authorization required',
		'需要 windows 管理员授权',
		'access is denied',
		'access denied',
		'拒绝访问',
		'系统错误 5',
	].some(marker => message.includes(marker))
}

async function grantMember(candidate: TaskTraceTeamMemberCandidate) {
	const accountName = candidate.account_name || candidate.username
	if (!accountName) return
	memberActionError.value = ''
	try {
		await teamStore.grantMember(candidate)
	} catch (cause) {
		if (!needsAdministratorAuthorization(cause)) {
			memberActionError.value = getErrorText(cause)
			error(cause)
			return
		}
		const confirmed = window.confirm(`为 ${accountName} 设置 teamData 共享读写权限需要 Windows 管理员授权。\n\n继续后 Windows 会显示用户账户控制窗口；本次操作完成后 TaskTrace 仍以普通权限运行。`)
		if (!confirmed) return
		try {
			await teamStore.grantMember(candidate, true)
		} catch (elevatedCause) {
			memberActionError.value = getErrorText(elevatedCause)
			error(elevatedCause)
			return
		}
	}
	addingMember.value = false
	success({message: `已为 ${accountName} 设置 teamData 读写权限。`})
}

async function setMaximumAccess(member: string, access: 'read' | 'write', elevate = false) {
	return teamStore.setMemberAccess(member, access, elevate)
}

async function changeMemberAccess(member: string, access: 'read' | 'write') {
	if (changingMember.value || teamStore.maximumAccessFor(member) === access) return
	changingMember.value = member
	memberActionError.value = ''
	try {
		try {
			await setMaximumAccess(member, access)
		} catch (cause) {
			if (!needsAdministratorAuthorization(cause)) throw cause
			const confirmed = window.confirm(`修改 ${member} 的 teamData ${access === 'write' ? '读写' : '仅读'}权限需要 Windows 管理员授权。\n\n继续后只会为本次操作短暂提升权限，TaskTrace 仍以普通权限运行。`)
			if (!confirmed) return
			await setMaximumAccess(member, access, true)
		}
		success({message: `已将 ${member} 的最高权限设置为${access === 'write' ? '读写' : '仅读'}。`})
	} catch (cause) {
		memberActionError.value = getErrorText(cause)
		error(cause)
	} finally {
		changingMember.value = ''
	}
}

function requestRemoveMember(member: string) {
	memberActionError.value = ''
	pendingRemoval.value = member
}

function cancelRemoveMember() {
	if (removingMember.value) return
	pendingRemoval.value = ''
}

async function confirmRemoveMember() {
	const member = pendingRemoval.value
	if (!member || removingMember.value) return
	removingMember.value = member
	memberActionError.value = ''
	try {
		try {
			await teamStore.removeMember(member)
		} catch (cause) {
			if (!needsAdministratorAuthorization(cause)) throw cause
			await teamStore.removeMember(member, true)
		}
		pendingRemoval.value = ''
		success({message: `已删除 ${member} 的 teamData 权限。`})
	} catch (cause) {
		try {
			await teamStore.refresh()
			if (!teamStore.status.unassigned_members?.some(value => teamStore.memberKey(value) === teamStore.memberKey(member))) {
				pendingRemoval.value = ''
				success({message: `已删除 ${member} 的 teamData 权限。`})
				return
			}
		} catch { /* Keep the original permission error when status cannot be refreshed. */ }
		memberActionError.value = getErrorText(cause)
		error(cause)
	} finally {
		removingMember.value = ''
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

.team-member-summary,
.team-member-actions {
	display: flex;
	align-items: center;
	gap: .6rem;
}

.team-member-actions label {
	color: var(--grey-600);
	font-size: .8rem;
}

.team-member-actions select {
	min-inline-size: 6rem;
	padding: .45rem .6rem;
	border: 1px solid var(--grey-300);
	border-radius: 6px;
	background: var(--white);
	color: var(--text);
}

.team-member-names {
	display: flex;
	flex-wrap: wrap;
	gap: .2rem .65rem;
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

.team-member-error {
	margin-block: .75rem 0;
	overflow-wrap: anywhere;
}
</style>
