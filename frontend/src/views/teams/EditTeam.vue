<template>
	<div
		class="loader-container is-max-width-desktop"
		:class="{ 'is-loading': teamService.loading }"
	>
		<Card
			v-if="userIsAdmin && !team.oidcId"
			class="is-fullwidth"
			:title="title"
		>
			<form @submit.prevent="save()">
				<FormField
					id="teamtext"
					v-model="team.name"
					v-focus
					:label="$t('team.attributes.name')"
					:disabled="teamMemberService.loading"
					:loading="teamMemberService.loading"
					:placeholder="$t('team.attributes.namePlaceholder')"
					type="text"
					:error="showErrorTeamnameRequired && team.name === '' ? $t('team.attributes.nameRequired') : null"
				/>
				<FormField
					v-if="configStore.publicTeamsEnabled"
					:label="$t('team.attributes.isPublic')"
				>
					<FancyCheckbox
						v-model="team.isPublic"
						:disabled="teamMemberService.loading || undefined"
						:class="{ 'disabled': teamService.loading }"
					>
						{{ $t('team.attributes.isPublicDescription') }}
					</FancyCheckbox>
				</FormField>
				<FormField :label="$t('team.attributes.description')">
					<Editor
						id="teamdescription"
						v-model="team.description"
						:class="{ disabled: teamService.loading }"
						:disabled="teamService.loading"
						:placeholder="$t('team.attributes.descriptionPlaceholder')"
					/>
				</FormField>

				<div class="field has-addons mbs-4">
					<div class="control is-fullwidth">
						<XButton
							:loading="teamService.loading"
							class="is-fullwidth"
							type="submit"
						>
							{{ $t('misc.save') }}
						</XButton>
					</div>
					<div class="control">
						<XButton
							:loading="teamService.loading"
							danger
							icon="trash-alt"
							:aria-label="$t('team.edit.delete.header')"
							@click="showDeleteModal = true"
						/>
					</div>
				</div>
			</form>
		</Card>

		<Card
			class="is-fullwidth has-overflow"
			:title="$t('team.edit.members')"
			:padding="false"
		>
			<form
				v-if="userIsAdmin && !team.oidcId"
				class="p-4"
				@submit.prevent="addUser"
			>
				<div v-if="isLocalBuild">
					<TeamMemberPicker
						:input-id="`edit-team-member-search-${teamId}`"
						label="搜索团队成员"
						:excluded="teamMemberNames"
						@select="selectWindowsMember"
					/>
					<div
						v-if="newWindowsMember"
						class="selected-directory-member"
						:title="teamMemberVerification(newWindowsMember)"
					>
						<strong>{{ teamMemberDisplayName(newWindowsMember) }}</strong>
						<span>工号：{{ teamMemberEmployeeId(newWindowsMember) }}</span>
						<span>邮箱：{{ newWindowsMember.email || '未提供' }}</span>
					</div>
				</div>
				<div class="field has-addons">
					<div
						v-if="!isLocalBuild"
						class="control is-expanded"
					>
						<Multiselect
							v-model="newMember"
							:loading="userService.loading"
							:placeholder="$t('team.edit.search')"
							:search-results="foundUsers"
							label="username"
							@search="findUser"
						>
							<template #searchResult="{option: user}">
								<User
									:avatar-size="24"
									:user="user"
									class="m-0"
								/>
							</template>
						</Multiselect>
					</div>
					<div class="control">
						<XButton
							icon="plus"
							:disabled="isLocalBuild && !newWindowsMember"
							@click="addUser"
						>
							{{ $t('team.edit.addUser') }}
						</XButton>
					</div>
				</div>
				<p
					v-if="showMustSelectUserError"
					class="help is-danger"
				>
					{{ $t('team.edit.mustSelectUser') }}
				</p>
			</form>
			<div class="has-horizontal-overflow">
				<table class="table has-actions is-striped is-hoverable is-fullwidth">
					<tbody>
						<tr
							v-for="m in sortedMembers"
							:key="m.id"
						>
							<td>
								<TeamMemberIdentity
									v-if="isLocalBuild"
									:username="m.username"
								/>
								<User
									v-else
									:avatar-size="24"
									:user="m"
									class="m-0"
								/>
							</td>
							<td>
								<template v-if="m.id === userInfo.id">
									<b class="is-success">You</b>
								</template>
							</td>
							<td class="type">
								<template v-if="m.admin">
									<span class="icon is-small">
										<Icon icon="lock" />
									</span>
									{{ $t('team.attributes.admin') }}
								</template>
								<template v-else>
									<span class="icon is-small">
										<Icon icon="user" />
									</span>
									{{ $t('team.attributes.member') }}
								</template>
							</td>
							<td
								v-if="userIsAdmin"
								class="actions"
							>
								<XButton
									v-if="m.id !== userInfo.id"
									:loading="teamMemberService.loading"
									class="mie-2"
									@click="() => toggleUserType(m)"
								>
									{{ m.admin ? $t('team.edit.makeMember') : $t('team.edit.makeAdmin') }}
								</XButton>
								<XButton
									v-if="m.id !== userInfo.id"
									:loading="teamMemberService.loading"
									danger
									icon="trash-alt"
									:aria-label="$t('team.edit.deleteUser.header')"
									@click="() => {memberToDelete = m; showUserDeleteModal = true}"
								/>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</Card>

		<XButton
			v-if="team && !team.externalId"
			class="is-fullwidth is-danger"
			@click="showLeaveModal = true"
		>
			{{ $t('team.edit.leave.title') }}
		</XButton>

		<!-- Leave team modal -->
		<Modal
			v-if="showLeaveModal"
			@close="showLeaveModal = false"
			@submit="leave()"
		>
			<template #header>
				<span>{{ $t('team.edit.leave.title') }}</span>
			</template>

			<template #text>
				<p>
					{{ $t('team.edit.leave.text1') }}<br>
					{{ $t('team.edit.leave.text2') }}
				</p>
			</template>
		</Modal>

		<!-- Team delete modal -->
		<Modal
			:enabled="showDeleteModal"
			@close="showDeleteModal = false"
			@submit="deleteTeam()"
		>
			<template #header>
				<span>{{ $t('team.edit.delete.header') }}</span>
			</template>

			<template #text>
				<p>
					{{ $t('team.edit.delete.text1') }}<br>
					{{ $t('team.edit.delete.text2') }}
				</p>
			</template>
		</Modal>

		<!-- User delete modal -->
		<Modal
			:enabled="showUserDeleteModal"
			@close="showUserDeleteModal = false"
			@submit="deleteMember()"
		>
			<template #header>
				<span>{{ $t('team.edit.deleteUser.header') }}</span>
			</template>

			<template #text>
				<p>
					{{ $t('team.edit.deleteUser.text1') }}<br>
					{{ $t('team.edit.deleteUser.text2') }}
				</p>
			</template>
		</Modal>
	</div>
</template>

<script lang="ts" setup>
import {computed, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useRoute, useRouter} from 'vue-router'

import Editor from '@/components/input/AsyncEditor'
import FancyCheckbox from '@/components/input/FancyCheckbox.vue'
import FormField from '@/components/input/FormField.vue'
import Multiselect from '@/components/input/Multiselect.vue'
import User from '@/components/misc/User.vue'
import TeamMemberIdentity from '@/components/tasks/partials/TeamMemberIdentity.vue'
import TeamMemberPicker from '@/components/tasks/partials/TeamMemberPicker.vue'

import {getDisplayName} from '@/models/user'
import TeamService from '@/services/team'
import TeamMemberService from '@/services/teamMember'
import TeamMemberModel from '@/models/teamMember'
import UserService from '@/services/user'

import {PERMISSIONS as Permissions} from '@/constants/permissions'

import {useTitle} from '@/composables/useTitle'
import {success} from '@/message'
import {useAuthStore} from '@/stores/auth'
import {useConfigStore} from '@/stores/config'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import type {TaskTraceTeamMemberCandidate} from '@/client/generated'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {
	teamMemberCandidatePayload,
	teamMemberDisplayName,
	teamMemberEmployeeId,
	teamMemberVerification,
} from '@/helpers/tasktraceTeamMembers'

import type {ITeam} from '@/modelTypes/ITeam'
import type {IUser} from '@/modelTypes/IUser'
import type {ITeamMember} from '@/modelTypes/ITeamMember'

const authStore = useAuthStore()
const configStore = useConfigStore()
const tasktraceTeamStore = useTasktraceTeamStore()
const route = useRoute()
const router = useRouter()
const {t} = useI18n({useScope: 'global'})

const userIsAdmin = computed(() => {
	return (
		team.value &&
		team.value.maxPermission &&
		team.value.maxPermission > Permissions.READ
	)
})
const userInfo = computed(() => authStore.info)

const sortedMembers = computed(() => {
	return [...(team.value?.members ?? [])].sort((a, b) =>
		getDisplayName(a).localeCompare(getDisplayName(b), undefined, {sensitivity: 'base'}),
	)
})

const teamService = ref<TeamService>(new TeamService())
const teamMemberService = ref<TeamMemberService>(new TeamMemberService())
const userService = ref<UserService>(new UserService())

const team = ref<ITeam>()
const teamId = computed(() => Number(route.params.id))
const memberToDelete = ref<ITeamMember>()
const newMember = ref<IUser>()
const newWindowsMember = ref<TaskTraceTeamMemberCandidate>()
const foundUsers = ref<IUser[]>()
const teamMemberNames = computed(() => (team.value?.members ?? []).map(member => member.username))

const showDeleteModal = ref(false)
const showUserDeleteModal = ref(false)
const showLeaveModal = ref(false)
const showErrorTeamnameRequired = ref(false)
const showMustSelectUserError = ref(false)

const title = ref('')

loadTeam()

async function loadTeam() {
	team.value = await teamService.value.get({id: teamId.value})
	if (isLocalBuild) {
		for (const member of team.value?.members ?? []) {
			void tasktraceTeamStore.searchMembers(member.username).catch(() => undefined)
		}
	}
	title.value = t('team.edit.title', {team: team.value?.name})
	useTitle(() => title.value)
}

async function save() {
	if (team.value?.name === '') {
		showErrorTeamnameRequired.value = true
		return
	}
	showErrorTeamnameRequired.value = false

	team.value = await teamService.value.update(team.value)
	success({message: t('team.edit.success')})
}

async function deleteTeam() {
	await teamService.value.delete(team.value)
	success({message: t('team.edit.delete.success')})
	router.push({name: 'teams.index'})
}

async function deleteMember() {
	try {
		await teamMemberService.value.delete({
			teamId: teamId.value,
			username: memberToDelete.value.username,
		})
		success({message: t('team.edit.deleteUser.success')})
		await loadTeam()
	} finally {
		showUserDeleteModal.value = false
	}
}

async function addUser() {
	showMustSelectUserError.value = false
	if(!newMember.value && !newWindowsMember.value) {
		showMustSelectUserError.value = true
		return
	}
	const member = newWindowsMember.value
		? new TeamMemberModel({...teamMemberCandidatePayload(newWindowsMember.value), teamId: teamId.value})
		: new TeamMemberModel({teamId: teamId.value, username: newMember.value!.username})
	await teamMemberService.value.create(member)
	newMember.value = null
	newWindowsMember.value = undefined
	await loadTeam()
	success({message: t('team.edit.userAddedSuccess')})
}

function selectWindowsMember(candidate: TaskTraceTeamMemberCandidate) {
	newWindowsMember.value = candidate
}

async function toggleUserType(member: ITeamMember) {
	// FIXME: direct manipulation
	member.admin = !member.admin
	member.teamId = teamId.value
	const r = await teamMemberService.value.update(member)
	for (const tm of team.value.members) {
		if (tm.id === member.id) {
			tm.admin = r.admin
			break
		}
	}
	success({
		message: member.admin ?
			t('team.edit.madeAdmin') :
			t('team.edit.madeMember'),
	})
}

async function findUser(query: string) {
	if (query === '') {
		foundUsers.value = []
		return
	}

	const users = await userService.value.getAll({}, {s: query})
	foundUsers.value = users.filter((u: IUser) => u.id !== userInfo.value.id)
}

async function leave() {
	try {
		await teamMemberService.value.delete({
			teamId: teamId.value,
			username: userInfo.value.username,
		})
		success({message: t('team.edit.leave.success')})
		await router.push({name: 'home'})
	} finally {
		showUserDeleteModal.value = false
	}
}
</script>

<style lang="scss" scoped>
.card.is-fullwidth {
	margin-block-end: 1rem;

	.content {
		padding: 0;
	}
}

.selected-directory-member {
	display: grid;
	grid-template-columns: minmax(8rem, 1fr) minmax(7rem, auto) minmax(10rem, auto);
	gap: 1rem;
	padding: .65rem .75rem;
	margin-block: .75rem;
	border: 1px solid var(--grey-200);
	border-radius: $radius;
	background: var(--grey-50);
}

.selected-directory-member span {
	color: var(--grey-500);
	font-size: .85rem;
	overflow-wrap: anywhere;
}

@media screen and (max-width: $tablet) {
	.selected-directory-member {
		grid-template-columns: 1fr;
		gap: .15rem;
	}
}
</style>
