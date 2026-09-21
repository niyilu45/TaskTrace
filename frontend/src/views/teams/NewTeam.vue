<template>
	<CreateEdit
		v-model:loading="loadingModel"
		:title="title"
		:primary-disabled="team.name.trim() === '' || duplicateName"
		@create="createTeam()"
	>
		<FormField
			id="teamName"
			v-model="team.name"
			v-focus
			:label="$t('team.attributes.name')"
			:disabled="teamService.loading"
			:loading="teamService.loading"
			:placeholder="$t('team.attributes.namePlaceholder')"
			type="text"
			:error="nameError"
			@update:modelValue="duplicateName = false"
			@keyup.enter="createTeam"
		/>
		<div
			v-if="isLocalBuild"
			class="field"
		>
			<TeamMemberPicker
				input-id="new-team-member-search"
				label="团队成员"
				:excluded="selectedMemberKeys"
				@select="addSelectedMember"
			/>
			<div
				v-if="selectedMembers.length"
				class="selected-members"
			>
				<button
					v-for="member in selectedMembers"
					:key="teamMemberKey(member.account_name || member.username)"
					type="button"
					class="selected-member"
					:title="`${teamMemberVerification(member)}；点击移除`"
					@click="removeSelectedMember(member)"
				>
					<span>{{ teamMemberDisplayName(member) }}</span>
					<small>{{ teamMemberEmployeeId(member) }}</small>
					<Icon icon="times" />
				</button>
			</div>
			<p class="help">
				点击输入框会立即列出已经添加过的协作人员。
			</p>
		</div>
		<FormField
			v-if="configStore.publicTeamsEnabled"
			:label="$t('team.attributes.isPublic')"
		>
			<FancyCheckbox
				v-model="team.isPublic"
				:class="{ 'disabled': teamService.loading }"
				:disabled="teamService.loading"
			>
				{{ $t('team.attributes.isPublicDescription') }}
			</FancyCheckbox>
		</FormField>
	</CreateEdit>
</template>

<script setup lang="ts">
import {computed, reactive, ref, shallowReactive} from 'vue'
import {useI18n} from 'vue-i18n'

import TeamModel from '@/models/team'
import TeamMemberModel from '@/models/teamMember'
import TeamService from '@/services/team'
import TeamMemberService from '@/services/teamMember'

import CreateEdit from '@/components/misc/CreateEdit.vue'
import FancyCheckbox from '@/components/input/FancyCheckbox.vue'
import FormField from '@/components/input/FormField.vue'
import TeamMemberPicker from '@/components/tasks/partials/TeamMemberPicker.vue'

import {useTitle} from '@/composables/useTitle'
import {useRouter} from 'vue-router'
import {error, success} from '@/message'

import {useConfigStore} from '@/stores/config'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import type {TaskTraceTeamMemberCandidate} from '@/client/generated'
import {
	teamMemberCandidatePayload,
	teamMemberDisplayName,
	teamMemberEmployeeId,
	teamMemberKey,
	teamMemberVerification,
} from '@/helpers/tasktraceTeamMembers'
import {isLocalBuild} from '@/helpers/tasktraceLocal'

defineOptions({name: 'NewTeam'})

const {t} = useI18n()
const title = computed(() => t('team.create.title'))
useTitle(title)
const router = useRouter()

const teamService = shallowReactive(new TeamService())
const teamMemberService = shallowReactive(new TeamMemberService())
const team = reactive(new TeamModel())
const selectedMembers = ref<TaskTraceTeamMemberCandidate[]>([])
const showError = ref(false)
const duplicateName = ref(false)
const isSubmitting = ref(false)

const loadingModel = computed({
	get: () => isSubmitting.value || teamService.loading,
	set(value: boolean) {
		isSubmitting.value = value
	},
})

const configStore = useConfigStore()
const teamStore = useTasktraceTeamStore()
const selectedMemberKeys = computed(() => [
	teamStore.status.username || '',
	...selectedMembers.value.map(member => member.account_name || member.username || ''),
])

function addSelectedMember(candidate: TaskTraceTeamMemberCandidate) {
	const key = teamMemberKey(candidate.account_name || candidate.username)
	if (!key || selectedMembers.value.some(member => teamMemberKey(member.account_name || member.username) === key)) return
	selectedMembers.value.push(candidate)
}

function removeSelectedMember(candidate: TaskTraceTeamMemberCandidate) {
	const key = teamMemberKey(candidate.account_name || candidate.username)
	selectedMembers.value = selectedMembers.value.filter(member => teamMemberKey(member.account_name || member.username) !== key)
}

const nameError = computed(() => {
	if (showError.value && team.name.trim() === '') return t('team.attributes.nameRequired')
	if (duplicateName.value) return t('team.attributes.nameDuplicate')
	return null
})

async function teamNameExists(name: string) {
	const normalized = name.toLocaleLowerCase()
	const matches = await teamService.getAll(new TeamModel(), {s: name})
	return matches.some(candidate => candidate.name.trim().toLocaleLowerCase() === normalized)
}

async function createTeam() {
	team.name = team.name.trim()
	if (team.name === '') {
		showError.value = true
		return
	}
	showError.value = false

	if (isSubmitting.value) {
		return
	}

	isSubmitting.value = true

	try {
		duplicateName.value = await teamNameExists(team.name)
		if (duplicateName.value) return
		const response = await teamService.create(team)
		const failedMembers: string[] = []
		for (const candidate of selectedMembers.value) {
			try {
				await teamMemberService.create(new TeamMemberModel({
					...teamMemberCandidatePayload(candidate),
					teamId: response.id,
				}))
			} catch {
				failedMembers.push(teamMemberDisplayName(candidate))
			}
		}
		await router.push({
			name: 'teams.edit',
			params: { id: response.id },
		})
		if (failedMembers.length) {
			error({message: `团队已创建，以下成员未能加入：${failedMembers.join('、')}。可以在团队页面重试。`})
		} else {
			success({message: t('team.create.success') })
		}
	} catch (cause) {
		if ((cause as {response?: {data?: {code?: number}}})?.response?.data?.code === 6011) {
			duplicateName.value = true
			return
		}
		throw cause
	} finally {
		isSubmitting.value = false
	}
}
</script>

<style scoped lang="scss">
.selected-members {
	display: flex;
	flex-wrap: wrap;
	gap: .5rem;
	margin-block-start: .75rem;
}

.selected-member {
	display: inline-flex;
	align-items: center;
	gap: .35rem;
	padding: .4rem .6rem;
	border: 1px solid var(--grey-200);
	border-radius: $radius;
	background: var(--white);
	color: var(--text);
	cursor: pointer;
}

.selected-member:hover {
	border-color: var(--danger);
}

.selected-member small {
	color: var(--grey-500);
}
</style>
