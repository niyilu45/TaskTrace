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
				<p><strong>协作成员：</strong>{{ binding.members?.join('、') }}</p>
				<p v-if="binding.last_sync" class="has-text-grey">最近同步：{{ formatDisplayDate(binding.last_sync) }}</p>
				<p v-if="binding.last_error" class="notification is-warning is-light">共享路径暂时不可用：{{ binding.last_error }}。本地修改已保留，重新连接后会自动合并。</p>
			</div>
			<div class="field">
				<label class="label" :for="`team-link-${taskId}`">任务链接</label>
				<div class="field has-addons">
					<div class="control is-expanded">
						<input :id="`team-link-${taskId}`" class="input" :value="binding.link" readonly>
					</div>
					<div class="control">
						<XButton variant="secondary" @click="copyLink">复制链接</XButton>
					</div>
				</div>
				<p class="help">接收者通过完整界面的“导入任务链接”添加，只能看到此任务和它的子任务、进展及遗留事项。</p>
			</div>
			<label class="checkbox team-notify">
				<input
					:checked="binding.notify"
					type="checkbox"
					@change="setNotify(($event.target as HTMLInputElement).checked)"
				>
				更新后通知其他协作成员
			</label>
			<p v-if="binding.conflicts?.length" class="notification is-danger is-light">
				检测到 {{ binding.conflicts.length }} 项冲突，请从顶部的团队通知入口一次性处理。
			</p>
		</template>

		<form v-else @submit.prevent="shareTask">
			<p>把当前任务及其全部子任务加入 teamData。父任务、同级任务和个人优先级不会共享。</p>
			<div v-if="candidateMembers.length" class="field">
				<span class="label">从 teamData 文件夹权限中发现的成员</span>
				<label
					v-for="member in candidateMembers"
					:key="member"
					class="checkbox team-member"
				>
					<input v-model="selectedMembers" type="checkbox" :value="member">
					{{ member }}
				</label>
			</div>
			<div class="field">
				<label class="label" :for="`team-members-${taskId}`">其他成员用户名</label>
				<input
					:id="`team-members-${taskId}`"
					v-model="manualMembers"
					class="input"
					placeholder="多个用户名用逗号分隔"
				>
				<p class="help">同一用户名在多台电脑上会被识别为同一个人。</p>
			</div>
			<div v-if="!teamStore.status.repository?.shared" class="notification is-warning is-light">
				尚未检测到 Windows 共享。请先在 teamData 文件夹属性中授予成员访问权限；程序重启后会自动识别共享路径和候选成员。
			</div>
			<XButton type="submit" variant="primary" :loading="teamStore.loading">共享当前任务</XButton>
		</form>
	</section>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'

import XButton from '@/components/input/Button.vue'
import {formatDisplayDate} from '@/helpers/time/formatDate'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {error, success} from '@/message'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

const props = defineProps<{taskId: number}>()
const teamStore = useTasktraceTeamStore()
const selectedMembers = ref<string[]>([])
const manualMembers = ref('')

const binding = computed(() => teamStore.bindingForTask(props.taskId))
const candidateMembers = computed(() => (teamStore.status.repository?.candidates ?? []).filter(member => member.toLowerCase() !== teamStore.status.username?.toLowerCase()))

onMounted(() => {
	if (!teamStore.loaded) teamStore.refresh().catch(() => undefined)
})

async function shareTask() {
	const manual = manualMembers.value.split(/[,，;；\n]/).map(value => value.trim()).filter(Boolean)
	const members = [...new Set([...selectedMembers.value, ...manual].filter(value => value.toLowerCase() !== teamStore.status.username?.toLowerCase()))]
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

async function copyLink() {
	if (!binding.value?.link) return
	await navigator.clipboard.writeText(binding.value.link)
	success({message: '任务链接已复制。'})
}

async function setNotify(notify: boolean) {
	if (!binding.value?.share_id) return
	try { await teamStore.configure(binding.value.share_id, notify) } catch (cause) { error(cause) }
}
</script>

<style scoped lang="scss">
.team-collaboration { padding: 1rem; border: 1px solid var(--grey-200); border-radius: 10px; background: var(--white); }
.team-summary { display: flex; flex-wrap: wrap; gap: .5rem 1.5rem; }
.team-member { display: inline-flex; align-items: center; gap: .35rem; margin-inline-end: 1rem; }
.team-notify { display: inline-flex; gap: .45rem; align-items: center; margin-block: .5rem 1rem; }
</style>
