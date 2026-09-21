<template>
	<Multiselect
		v-model="assignees"
		class="edit-assignees"
		:class="{'has-assignees': assignees.length > 0}"
		:loading="loading"
		:placeholder="$t('task.assignee.placeholder')"
		:multiple="true"
		:disabled="disabled || (teamMode && !canManageTeamAssignees)"
		:search-results="foundUsers"
		:show-empty="true"
		label="name"
		:select-placeholder="$t('task.assignee.selectPlaceholder')"
		:autocomplete-enabled="false"
		@search="findUser"
		@select="addAssignee"
		@focus="preloadUsers"
	>
		<template #items="{items}">
			<AssigneeList
				:assignees="items"
				:disabled="disabled || (teamMode && !canManageTeamAssignees)"
				can-remove
				@remove="removeAssignee"
			/>
		</template>
		<template #searchResult="{option: user}">
			<User
				:avatar-size="24"
				:show-username="true"
				:user="user"
			/>
		</template>
	</Multiselect>
</template>

<script setup lang="ts">
import {computed, ref, shallowReactive, watch, nextTick} from 'vue'
import {useI18n} from 'vue-i18n'

import User from '@/components/misc/User.vue'
import Multiselect from '@/components/input/Multiselect.vue'

import {includesById} from '@/helpers/utils'
import ProjectUserService from '@/services/projectUsers'
import {success} from '@/message'
import {useAuthStore} from '@/stores/auth'
import {useTaskStore} from '@/stores/tasks'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

import type {IUser} from '@/modelTypes/IUser'
import {getDisplayName} from '@/models/user'
import AssigneeList from '@/components/tasks/partials/AssigneeList.vue'
import UserModel from '@/models/user'

const props = withDefaults(defineProps<{
	modelValue: IUser[] | undefined,
	taskId: number,
	projectId: number,
	disabled?: boolean,
}>(), {
	disabled: false,
})

const emit = defineEmits<{
	'update:modelValue': [value: IUser[] | undefined],
}>()

const authStore = useAuthStore()
const taskStore = useTaskStore()
const teamStore = useTasktraceTeamStore()
const {t} = useI18n({useScope: 'global'})

const projectUserService = shallowReactive(new ProjectUserService())
const foundUsers = ref<IUser[]>([])
const assignees = ref<IUser[]>([])
let isAdding = false
const binding = computed(() => teamStore.bindingForTask(props.taskId))
const teamTarget = computed(() => binding.value?.permission_targets?.find(target => target.kind === 'task' && target.task_id === props.taskId))
const teamMode = computed(() => Boolean(binding.value))
const canManageTeamAssignees = computed(() => teamTarget.value?.can_manage === true)
const loading = computed(() => projectUserService.loading || teamStore.loading)

function teamUser(username: string) {
	let hash = 0
	for (const char of username.toLocaleLowerCase()) hash = ((hash << 5) - hash + char.charCodeAt(0)) | 0
	return new UserModel({id: -(Math.abs(hash) + 1), username, name: username})
}

function syncTeamAssignees() {
	if (!teamMode.value) return
	assignees.value = (teamTarget.value?.permissions ?? [])
		.filter(permission => permission.assignee && permission.username)
		.map(permission => teamUser(permission.username!))
}

let hasPreloaded = false

function preloadUsers() {
	if (hasPreloaded) return
	hasPreloaded = true
	findUser()
}

watch(
	() => props.modelValue,
	(value) => {
		if (!teamMode.value) assignees.value = value ?? []
	},
	{
		immediate: true,
		deep: true,
	},
)

watch([teamMode, teamTarget], syncTeamAssignees, {immediate: true, deep: true})

async function addAssignee(user: IUser) {
	if (isAdding) {
		return
	}

	try {
		nextTick(() => isAdding = true)
		if (teamMode.value && binding.value?.share_id) {
			if (!canManageTeamAssignees.value) return
			const usernames = [...new Set([...assignees.value.map(item => item.username), user.username].filter(Boolean))]
			await teamStore.configureAssignees(binding.value.share_id, props.taskId, usernames)
			syncTeamAssignees()
			success({message: t('task.assignee.assignSuccess')})
			return
		}

		await taskStore.addAssignee({user: user, taskId: props.taskId})
		emit('update:modelValue', assignees.value)
		success({message: t('task.assignee.assignSuccess')})
	} finally {
		nextTick(() => isAdding = false)
	}
}

async function removeAssignee(user: IUser) {
	if (teamMode.value && binding.value?.share_id) {
		if (!canManageTeamAssignees.value) return
		await teamStore.configureAssignees(binding.value.share_id, props.taskId, assignees.value.filter(item => item.username.toLocaleLowerCase() !== user.username.toLocaleLowerCase()).map(item => item.username))
		syncTeamAssignees()
		success({message: t('task.assignee.unassignSuccess')})
		return
	}
	await taskStore.removeAssignee({user: user, taskId: props.taskId})

	// Remove the assignee from the project
	const idx = assignees.value.findIndex(a => a.id === user.id)
	if (idx !== -1) {
		assignees.value.splice(idx, 1)
	}
	success({message: t('task.assignee.unassignSuccess')})
}

async function findUser(query = '') {
	if (teamMode.value) {
		const selected = new Set(assignees.value.map(user => user.username.toLocaleLowerCase()))
		const keyword = query.trim().toLocaleLowerCase()
		foundUsers.value = teamStore.memberRoster
			.filter(username => !selected.has(username.toLocaleLowerCase()) && (!keyword || username.toLocaleLowerCase().includes(keyword)))
			.map(teamUser)
		return
	}
	const response = await projectUserService.getAll({projectId: props.projectId}, {s: query}) as IUser[]

	const currentUserId = authStore.info?.id

	// Filter the results to not include users who are already assigned
	foundUsers.value = response
		.filter(({id}) => !includesById(assignees.value, id))
		.map(u => {
			// Users may not have a display name set, so we fall back on the username in that case
			u.name = getDisplayName(u)
			return u
		})
		.sort((a, b) => {
			if (a.id === currentUserId) return -1
			if (b.id === currentUserId) return 1
			return a.name.localeCompare(b.name)
		})
}
</script>

<style lang="scss">
.edit-assignees.has-assignees.multiselect .input {
	padding-inline-start: 0;
}
</style>
