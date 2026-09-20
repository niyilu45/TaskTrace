<template>
	<section
		v-if="isLocalBuild && teamStore.status.enabled"
		class="content details team-collaboration d-print-none"
	>
		<h2 class="task-section-title">
			<span class="icon is-grey"><Icon icon="users" /></span>
			团队协作
		</h2>

		<template v-if="binding">
			<div class="team-summary">
				<div class="team-members">
					<strong>协作成员：</strong>
					<span
						v-for="member in binding.members"
						:key="member"
						class="team-member-chip"
					>
						<img
							v-if="avatarFor(member)"
							:src="avatarFor(member)"
							alt=""
							class="team-member-avatar"
						>
						<span
							v-else
							class="team-member-avatar team-member-avatar--fallback"
						>{{ initials(member) }}</span>
						{{ member }}
					</span>
				</div>
				<p
					v-if="binding.last_sync"
					class="has-text-grey"
				>
					最近同步：{{ formatDisplayDate(binding.last_sync) }}
				</p>
				<p
					v-if="binding.last_error"
					class="notification is-warning is-light"
				>
					共享路径暂时不可用：{{ binding.last_error }}。本地修改已保留，重新连接后会自动合并。
				</p>
				<XButton
					v-if="binding.member_link"
					variant="secondary"
					@click="copyMemberLink"
				>
					分享成员
				</XButton>
			</div>
			<div class="field">
				<label
					class="label"
					:for="`team-link-${taskId}`"
				>任务链接</label>
				<div class="field has-addons">
					<div class="control is-expanded">
						<input
							:id="`team-link-${taskId}`"
							class="input"
							:value="binding.link"
							readonly
						>
					</div>
					<div class="control">
						<XButton
							type="button"
							variant="secondary"
							@click="copyLink"
						>
							复制链接
						</XButton>
					</div>
				</div>
				<p class="help">
					接收者通过完整界面的“导入任务链接”添加，只能看到此任务和它的子任务、进展及遗留事项。
				</p>
			</div>
			<label class="checkbox team-notify">
				<input
					:checked="binding.notify"
					type="checkbox"
					@change="setNotify(($event.target as HTMLInputElement).checked)"
				>
				更新后通知其他协作成员
			</label>
			<p
				v-if="binding.conflicts?.length"
				class="notification is-danger is-light"
			>
				检测到 {{ binding.conflicts.length }} 项冲突，请从顶部的团队通知入口一次性处理。
			</p>
		</template>

		<form
			v-else
			@submit.prevent="shareTask"
		>
			<p>把当前任务及其全部子任务加入 teamData。父任务、同级任务和个人优先级不会共享。</p>
			<div
				v-if="candidateMembers.length"
				class="field"
			>
				<span class="label">从 teamData 文件夹权限中发现的成员</span>
				<label
					v-for="member in candidateMembers"
					:key="member"
					class="checkbox team-member"
				>
					<input
						v-model="selectedMembers"
						type="checkbox"
						:value="member"
					>
					{{ member }}
				</label>
			</div>
			<div class="field">
				<TeamMemberPicker
					:input-id="`team-members-${taskId}`"
					:excluded="[teamStore.status.username || '', ...selectedMembers]"
					@select="addMember"
				/>
				<div
					v-if="selectedMembers.length"
					class="team-selected-members"
				>
					<button
						v-for="member in selectedMembers"
						:key="member"
						type="button"
						class="team-member-chip team-member-chip--remove"
						:title="`移除 ${member}`"
						@click="removeSelected(member)"
					>
						{{ member }} ×
					</button>
				</div>
			</div>
			<div class="field">
				<label
					class="label"
					:for="`team-member-link-${taskId}`"
				>通过成员链接创建团队</label>
				<div class="field has-addons">
					<div class="control is-expanded">
						<input
							:id="`team-member-link-${taskId}`"
							v-model="memberLink"
							class="input"
							placeholder="粘贴 tasktrace-team-members:// 链接"
						>
					</div>
					<div class="control">
						<XButton
							type="button"
							variant="secondary"
							:loading="teamStore.loading"
							@click="importMembersForShare"
						>
							导入成员
						</XButton>
					</div>
				</div>
				<p class="help">
					有效成员会加入上方列表并自动获得 teamData 读写权限；失效成员会单独提示。
				</p>
			</div>
			<div
				v-if="!teamStore.status.repository?.shared"
				class="notification is-warning is-light"
			>
				尚未检测到 Windows 共享。添加成员时程序会明确提示需要先创建 teamData 共享。
			</div>
			<XButton
				type="submit"
				variant="primary"
				:loading="teamStore.loading"
			>
				共享当前任务
			</XButton>
		</form>
	</section>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'

import XButton from '@/components/input/Button.vue'
import TeamMemberPicker from '@/components/tasks/partials/TeamMemberPicker.vue'
import type {TaskTraceTeamMemberCandidate} from '@/client/generated'
import {formatDisplayDate} from '@/helpers/time/formatDate'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {error, success} from '@/message'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

const props = defineProps<{taskId: number}>()
const teamStore = useTasktraceTeamStore()
const selectedMembers = ref<string[]>([])
const memberLink = ref('')

const binding = computed(() => teamStore.bindingForTask(props.taskId))
const candidateMembers = computed(() => (teamStore.status.repository?.candidates ?? []).filter(member => member.toLowerCase() !== teamStore.status.username?.toLowerCase()))

function avatarFor(username: string) {
	return teamStore.status.profiles?.find(profile => profile.username?.toLowerCase() === username.toLowerCase())?.avatar || ''
}

function initials(username: string) {
	return username.trim().slice(0, 2).toUpperCase() || '?'
}

onMounted(() => {
	if (!teamStore.loaded) teamStore.refresh().catch(() => undefined)
})

async function shareTask() {
	const members = [...new Set(selectedMembers.value.filter(value => value.toLowerCase() !== teamStore.status.username?.toLowerCase()))]
	if (!members.length) {
		error({message: '请至少选择或填写一位其他成员。'})
		return
	}
	try {
		await teamStore.share(props.taskId, members)
		success({message: '团队任务已创建，请把任务链接发给协作成员。'})
	} catch (cause) {
		error(cause)
	}
}

function addMember(candidate: TaskTraceTeamMemberCandidate) {
	const member = candidate.account_name || candidate.username
	if (member && !selectedMembers.value.includes(member)) selectedMembers.value.push(member)
}

function removeSelected(member: string) {
	selectedMembers.value = selectedMembers.value.filter(value => value !== member)
}

async function importMembersForShare() {
	if (!memberLink.value.trim()) {
		error({message: '请先粘贴团队成员链接。'})
		return
	}
	try {
		const result = await teamStore.importMembers(memberLink.value.trim())
		for (const member of result.added ?? []) {
			if (!selectedMembers.value.includes(member)) selectedMembers.value.push(member)
		}
		memberLink.value = ''
		const failed = result.failed ?? []
		if (failed.length) {
			error({message: `已导入其余有效成员；以下成员失败：${failed.map(item => item.username).join('、')}`})
		} else {
			success({message: '成员已导入，可以创建团队任务。'})
		}
	} catch (cause) {
		error(cause)
	}
}

async function copyLink() {
	if (!binding.value?.link) return
	await navigator.clipboard.writeText(binding.value.link)
	success({message: '任务链接已复制。'})
}

async function copyMemberLink() {
	if (!binding.value?.member_link) return
	await navigator.clipboard.writeText(binding.value.member_link)
	success({message: '团队成员链接已复制。'})
}

async function setNotify(notify: boolean) {
	if (!binding.value?.share_id) return
	try { await teamStore.configure(binding.value.share_id, notify) } catch (cause) { error(cause) }
}
</script>

<style scoped lang="scss">
.team-collaboration {
	padding: 1rem;
	border: 1px solid var(--grey-200);
	border-radius: 10px;
	background: var(--white);
}

.team-summary {
	display: flex;
	flex-wrap: wrap;
	gap: .5rem 1.5rem;
}

.team-members {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: .45rem;
}

.team-member-chip {
	display: inline-flex;
	align-items: center;
	gap: .35rem;
	padding: .2rem .5rem .2rem .25rem;
	border: 1px solid var(--grey-200);
	border-radius: 999px;
	background: var(--grey-50);
}

.team-member-chip--remove {
	color: var(--text);
	cursor: pointer;
}

.team-selected-members {
	display: flex;
	flex-wrap: wrap;
	gap: .4rem;
	margin-block-start: .5rem;
}

.team-member-avatar {
	inline-size: 24px;
	block-size: 24px;
	border-radius: 50%;
	object-fit: cover;
}

.team-member-avatar--fallback {
	display: grid;
	place-items: center;
	background: var(--primary);
	color: var(--white);
	font-size: .6rem;
	font-weight: 700;
}

.team-member {
	display: inline-flex;
	align-items: center;
	gap: .35rem;
	margin-inline-end: 1rem;
}

.team-notify {
	display: inline-flex;
	gap: .45rem;
	align-items: center;
	margin-block: .5rem 1rem;
}
</style>
