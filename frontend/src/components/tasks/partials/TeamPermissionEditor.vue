<template>
	<div
		class="permission-overlay"
		role="presentation"
		@click.self="$emit('close')"
	>
		<section
			class="permission-card"
			role="dialog"
			aria-modal="true"
			aria-labelledby="team-permission-title"
		>
			<header class="permission-card__header">
				<div>
					<h3 id="team-permission-title">
						协作读写权限
					</h3>
					<p>所属人和受理人始终具有读写权限，其他成员默认只读。</p>
				</div>
				<button
					type="button"
					class="delete"
					aria-label="关闭权限设置"
					@click="$emit('close')"
				/>
			</header>

			<div
				v-if="targets.length"
				class="permission-card__body"
			>
				<nav
					class="permission-targets"
					aria-label="权限对象"
				>
					<button
						v-for="target in targets"
						:key="targetKey(target)"
						type="button"
						:class="{'is-active': targetKey(target) === activeKey}"
						@click="selectTarget(target)"
					>
						<span>{{ target.kind === 'task' ? '任务' : '遗留事项' }}</span>
						<strong>{{ target.title || '未命名事项' }}</strong>
					</button>
				</nav>

				<div
					v-if="activeTarget"
					class="permission-members"
				>
					<div class="permission-members__title">
						<span>{{ activeTarget.kind === 'task' ? '任务' : '遗留事项' }}</span>
						<strong>{{ activeTarget.title || '未命名事项' }}</strong>
					</div>
					<p
						v-if="!canManageTarget"
						class="notification is-info is-light"
					>
						只有任务所属人或协作创建者可以修改权限；你可以查看当前配置。
					</p>
					<div class="permission-table-wrap">
						<table class="permission-table">
							<thead>
								<tr>
									<th scope="col">
										成员
									</th>
									<th scope="col">
										身份
									</th>
									<th scope="col">
										可读
									</th>
									<th scope="col">
										可写
									</th>
								</tr>
							</thead>
							<tbody>
								<tr
									v-for="permission in draftPermissions"
									:key="permission.username.toLowerCase()"
								>
									<td>
										<span class="permission-member">
											<img
												v-if="avatarFor(permission.username)"
												:src="avatarFor(permission.username)"
												alt=""
											>
											<span
												v-else
												class="permission-member__avatar"
											>{{ initials(permission.username) }}</span>
											{{ permission.username }}
										</span>
									</td>
									<td>
										<span
											v-if="permission.owner"
											class="tag is-primary is-light"
										>所属人</span>
										<span
											v-if="permission.assignee"
											class="tag is-success is-light"
										>受理人</span>
										<span
											v-if="!permission.owner && !permission.assignee"
											class="tag is-light"
										>团队成员</span>
									</td>
									<td>
										<input
											type="checkbox"
											:checked="permission.read"
											:disabled="locked(permission)"
											:aria-label="`${permission.username} 的读权限`"
											@change="setRead(permission, ($event.target as HTMLInputElement).checked)"
										>
									</td>
									<td>
										<input
											type="checkbox"
											:checked="permission.write"
											:disabled="locked(permission)"
											:aria-label="`${permission.username} 的写权限`"
											@change="setWrite(permission, ($event.target as HTMLInputElement).checked)"
										>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</div>
			</div>
			<p
				v-else
				class="permission-empty"
			>
				权限信息正在同步，请稍后重新打开。
			</p>

			<footer class="permission-card__footer">
				<XButton
					variant="secondary"
					@click="$emit('close')"
				>
					关闭
				</XButton>
				<XButton
					v-if="canManageTarget && activeTarget"
					variant="primary"
					:loading="saving"
					:disabled="!changed"
					@click="save"
				>
					保存权限
				</XButton>
			</footer>
		</section>
	</div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref, watch} from 'vue'

import XButton from '@/components/input/Button.vue'
import type {
	TaskTraceTeamBindingStatus,
	TaskTraceTeamPermissionTarget,
} from '@/client/generated'
import {error, success} from '@/message'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

type EditablePermission = {
	username: string
	read: boolean
	write: boolean
	owner: boolean
	assignee: boolean
}

const props = defineProps<{
	taskId: number
	binding: TaskTraceTeamBindingStatus
}>()

const emit = defineEmits<{close: []}>()
const teamStore = useTasktraceTeamStore()
const activeKey = ref('')
const draftPermissions = ref<EditablePermission[]>([])
const original = ref('')
const saving = ref(false)

const targets = computed(() => (props.binding.permission_targets ?? [])
	.filter(target => target.task_id === props.taskId)
	.sort((left, right) => left.kind === right.kind ? 0 : left.kind === 'task' ? -1 : 1))

const activeTarget = computed(() => targets.value.find(target => targetKey(target) === activeKey.value))
const canManageTarget = computed(() => activeTarget.value?.can_manage === true)
const changed = computed(() => signature(draftPermissions.value) !== original.value)

function targetKey(target: TaskTraceTeamPermissionTarget) {
	return `${target.node_id || ''}:${target.outstanding_id || ''}`
}

function signature(permissions: EditablePermission[]) {
	return JSON.stringify(permissions.map(permission => ({
		username: permission.username.toLowerCase(),
		read: permission.read,
		write: permission.write,
	})))
}

function loadTarget(target?: TaskTraceTeamPermissionTarget) {
	if (!target) {
		activeKey.value = ''
		draftPermissions.value = []
		original.value = ''
		return
	}
	activeKey.value = targetKey(target)
	draftPermissions.value = (target.permissions ?? []).map(permission => ({
		username: permission.username || '',
		read: Boolean(permission.read || permission.write),
		write: Boolean(permission.write),
		owner: Boolean(permission.owner),
		assignee: Boolean(permission.assignee),
	}))
	original.value = signature(draftPermissions.value)
}

function selectTarget(target: TaskTraceTeamPermissionTarget) {
	if (changed.value && !window.confirm('当前权限尚未保存，确定切换吗？')) return
	loadTarget(target)
}

function locked(permission: EditablePermission) {
	return !canManageTarget.value || permission.owner || permission.assignee || saving.value
}

function setRead(permission: EditablePermission, value: boolean) {
	permission.read = value
	if (!value) permission.write = false
}

function setWrite(permission: EditablePermission, value: boolean) {
	permission.write = value
	if (value) permission.read = true
}

function avatarFor(username: string) {
	return teamStore.status.profiles?.find(profile => profile.username?.toLowerCase() === username.toLowerCase())?.avatar || ''
}

function initials(username: string) {
	return username.trim().slice(0, 2).toUpperCase() || '?'
}

async function save() {
	const target = activeTarget.value
	if (!target || !props.binding.share_id || !target.task_id || saving.value) return
	saving.value = true
	try {
		await teamStore.configurePermissions(
			props.binding.share_id,
			target.task_id,
			target.outstanding_id || '',
			draftPermissions.value.map(permission => ({
				username: permission.username,
				read: permission.read,
				write: permission.write,
			})),
		)
		const refreshedBinding = teamStore.bindingForTask(props.taskId)
		const refreshedTarget = refreshedBinding?.permission_targets?.find(item => targetKey(item) === activeKey.value)
		loadTarget(refreshedTarget)
		success({message: target.kind === 'task' ? '协作权限已保存，并已同步到子任务和遗留事项。' : '协作权限已保存。'})
	} catch (cause) {
		error(cause)
	} finally {
		saving.value = false
	}
}

function closeOnEscape(event: KeyboardEvent) {
	if (event.key !== 'Escape') return
	if (changed.value && !window.confirm('权限尚未保存，确定关闭吗？')) return
	emit('close')
}

watch(targets, value => {
	const current = value.find(target => targetKey(target) === activeKey.value)
	loadTarget(current ?? value[0])
}, {immediate: true})

onMounted(() => window.addEventListener('keydown', closeOnEscape))
onBeforeUnmount(() => window.removeEventListener('keydown', closeOnEscape))
</script>

<style scoped lang="scss">
.permission-overlay {
	position: fixed;
	z-index: 110;
	inset: 0;
	display: grid;
	place-items: center;
	padding: 1rem;
	background: rgb(17 24 39 / 48%);
}

.permission-card {
	display: flex;
	flex-direction: column;
	inline-size: min(920px, 100%);
	max-block-size: min(760px, calc(100vh - 2rem));
	overflow: hidden;
	border-radius: 12px;
	background: var(--white);
	box-shadow: 0 20px 55px rgb(17 24 39 / 25%);
}

.permission-card__header,
.permission-card__footer {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 1rem;
	padding: 1rem 1.25rem;
	border-block-end: 1px solid var(--grey-200);
}

.permission-card__header {
	h3 {
		margin: 0;
		font-size: 1.2rem;
	}

	p {
		margin: .2rem 0 0;
		color: var(--grey-600);
	}
}

.permission-card__footer {
	justify-content: flex-end;
	border-block-start: 1px solid var(--grey-200);
	border-block-end: 0;
}

.permission-card__body {
	display: grid;
	grid-template-columns: minmax(180px, 240px) minmax(0, 1fr);
	min-block-size: 0;
	overflow: hidden;
}

.permission-targets {
	overflow: auto;
	padding: .75rem;
	border-inline-end: 1px solid var(--grey-200);
	background: var(--grey-50);

	button {
		display: flex;
		flex-direction: column;
		gap: .15rem;
		inline-size: 100%;
		margin-block-end: .4rem;
		padding: .65rem .75rem;
		border: 1px solid transparent;
		border-radius: 8px;
		background: transparent;
		text-align: start;
		cursor: pointer;

		span {
			color: var(--grey-600);
			font-size: .75rem;
		}

		strong {
			overflow: hidden;
			text-overflow: ellipsis;
			white-space: nowrap;
		}

		&:hover,
		&.is-active {
			border-color: var(--primary);
			background: var(--white);
		}
	}
}

.permission-members {
	min-inline-size: 0;
	overflow: auto;
	padding: 1rem;
}

.permission-members__title {
	display: flex;
	align-items: baseline;
	gap: .5rem;
	margin-block-end: .75rem;

	span {
		color: var(--grey-600);
		font-size: .8rem;
	}
}

.permission-table-wrap {
	overflow: auto;
}

.permission-table {
	inline-size: 100%;
	border-collapse: collapse;

	th,
	td {
		padding: .65rem;
		border-block-end: 1px solid var(--grey-200);
		text-align: start;
		vertical-align: middle;
	}

	th:nth-last-child(-n + 2),
	td:nth-last-child(-n + 2) {
		inline-size: 4.5rem;
		text-align: center;
	}
}

.permission-member {
	display: inline-flex;
	align-items: center;
	gap: .45rem;

	img,
	.permission-member__avatar {
		inline-size: 1.75rem;
		block-size: 1.75rem;
		border-radius: 50%;
	}

	img {
		object-fit: cover;
	}
}

.permission-member__avatar {
	display: inline-grid;
	place-items: center;
	background: var(--primary);
	color: var(--white);
	font-size: .65rem;
	font-weight: 700;
}

.permission-table .tag + .tag {
	margin-inline-start: .3rem;
}

.permission-empty {
	padding: 2rem;
	text-align: center;
}

@media (width <= 700px) {
	.permission-card__body {
		grid-template-columns: 1fr;
		overflow: auto;
	}

	.permission-targets {
		display: flex;
		overflow-x: auto;
		border-inline-end: 0;
		border-block-end: 1px solid var(--grey-200);

		button {
			flex: 0 0 180px;
			margin: 0 .4rem 0 0;
		}
	}
}
</style>
