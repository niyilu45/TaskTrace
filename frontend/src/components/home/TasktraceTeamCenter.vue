<template>
	<div
		v-if="isLocalBuild"
		class="team-center"
		:class="{'team-center--page': !activityHost, 'team-center--activity-host': activityHost}"
	>
		<template v-if="!activityHost">
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
						:disabled="reconcilingMembers || !!grantingMember || !!changingMember || !!removingMember"
						@click="addingMember = !addingMember"
					>
						{{ addingMember ? '取消添加' : '添加协作成员' }}
					</XButton>
				</div>
				<TeamMemberPicker
					v-if="addingMember && !grantingMember"
					input-id="team-unassigned-member-search"
					:excluded="knownMembers"
					@select="grantMember"
				/>
				<p
					v-if="reconcilingMembers || grantingMember || memberActionSuccess"
					class="team-member-feedback"
					role="status"
				>
					{{ reconcilingMembers ? '正在补全已有协作成员的 teamData 共享权限…' : grantingMember ? `正在为 ${grantingMember} 添加协作权限…` : memberActionSuccess }}
				</p>
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
								:disabled="reconcilingMembers || member.isCurrent || changingMember === member.username"
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
								:disabled="reconcilingMembers || !!removingMember || !!changingMember"
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
					:disabled="reconcilingMembers"
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

		<TeamConflictResolver
			v-if="activityHost && showConflicts"
			@close="showConflicts = false"
		/>
	</div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref} from 'vue'
import {useVisiblePolling} from '@/composables/useVisiblePolling'

import TeamConflictResolver from './TeamConflictResolver.vue'
import Modal from '@/components/misc/Modal.vue'
import TeamMemberPicker from '@/components/tasks/partials/TeamMemberPicker.vue'
import TeamMemberIdentity from '@/components/tasks/partials/TeamMemberIdentity.vue'
import XButton from '@/components/input/Button.vue'
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
const showConflicts = ref(false)
const addingMember = ref(false)
const memberLink = ref('')
const memberImportResult = ref<TaskTraceTeamMemberImportResult | null>(null)
const pendingRemoval = ref('')
const removingMember = ref('')
const changingMember = ref('')
const memberActionError = ref('')
const memberActionSuccess = ref('')
const grantingMember = ref('')
const reconcilingMembers = ref(false)
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
	if (!activityHost.value) void refreshMembers()
	if (activityHost.value) {
		window.addEventListener('tasktrace-team-conflicts-open', openConflicts)
	}
})

onBeforeUnmount(() => {
	window.removeEventListener('tasktrace-team-conflicts-open', openConflicts)
})

function openConflicts() {
	showConflicts.value = true
}

async function refreshMembers() {
	try {
		await teamStore.refresh(true, true)
		// Only show progress when the server has identified permissions to repair.
		reconcilingMembers.value = Boolean(teamStore.status.repository?.pending_members?.length)
		const added = await teamStore.reconcileMemberAccess()
		if (added.length) memberActionSuccess.value = `已为 ${added.length} 位已有协作成员补全本机 teamData 共享权限。`
	} catch (cause) {
		memberActionError.value = getErrorText(cause)
	} finally {
		reconcilingMembers.value = false
	}
}

async function grantMember(candidate: TaskTraceTeamMemberCandidate) {
	const accountName = candidate.account_name || candidate.username
	if (!accountName || grantingMember.value || reconcilingMembers.value) return
	memberActionError.value = ''
	memberActionSuccess.value = ''
	grantingMember.value = teamStore.displayNameFor(accountName) || accountName
	try {
		if (!await teamStore.grantMemberWithConsent(candidate)) return
		addingMember.value = false
		memberActionSuccess.value = `已成功添加 ${teamStore.displayNameFor(accountName)}，并设置 teamData 读写权限。`
		success({message: memberActionSuccess.value})
	} catch (cause) {
		memberActionError.value = getErrorText(cause)
		error(cause)
	} finally {
		grantingMember.value = ''
	}
}

async function changeMemberAccess(member: string, access: 'read' | 'write') {
	if (reconcilingMembers.value || changingMember.value || teamStore.maximumAccessFor(member) === access) return
	changingMember.value = member
	memberActionError.value = ''
	memberActionSuccess.value = ''
	try {
		if (!await teamStore.setMemberAccessWithConsent(member, access)) return
		memberActionSuccess.value = `已将 ${teamStore.displayNameFor(member)} 的最高权限设置为${access === 'write' ? '读写' : '仅读'}。`
		success({message: memberActionSuccess.value})
	} catch (cause) {
		memberActionError.value = getErrorText(cause)
		error(cause)
	} finally {
		changingMember.value = ''
	}
}

function requestRemoveMember(member: string) {
	if (reconcilingMembers.value) return
	memberActionError.value = ''
	memberActionSuccess.value = ''
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
		if (!await teamStore.removeMemberWithConsent(member)) return
		pendingRemoval.value = ''
		success({message: `已删除 ${member} 的 teamData 权限。`})
	} catch (cause) {
		try {
			await teamStore.refresh(true)
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
	if (reconcilingMembers.value) return
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

</script>

<style scoped lang="scss">
.team-center {
	display: block;
}

.team-center--activity-host {
	display: contents;
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

.team-member-feedback {
	margin-block: .75rem 0;
	color: var(--text);
	overflow-wrap: anywhere;
}

.team-member-error {
	margin-block: .75rem 0;
	overflow-wrap: anywhere;
}
</style>
