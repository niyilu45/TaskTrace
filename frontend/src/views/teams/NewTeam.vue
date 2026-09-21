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
import TeamService from '@/services/team'

import CreateEdit from '@/components/misc/CreateEdit.vue'
import FancyCheckbox from '@/components/input/FancyCheckbox.vue'
import FormField from '@/components/input/FormField.vue'

import {useTitle} from '@/composables/useTitle'
import {useRouter} from 'vue-router'
import {success} from '@/message'

import {useConfigStore} from '@/stores/config'

defineOptions({name: 'NewTeam'})

const {t} = useI18n()
const title = computed(() => t('team.create.title'))
useTitle(title)
const router = useRouter()

const teamService = shallowReactive(new TeamService())
const team = reactive(new TeamModel())
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
		router.push({
			name: 'teams.edit',
			params: { id: response.id },
		})
		success({message: t('team.create.success') })
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
