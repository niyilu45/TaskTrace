<template>
	<div class="content has-text-centered">
		<h1 v-if="salutation">
			{{ salutation }}
		</h1>

		<Message
			v-if="deletionScheduledAt !== null"
			variant="danger"
			class="mbe-4"
		>
			{{
				$t('user.deletion.scheduled', {
					date: formatDisplayDate(deletionScheduledAt),
					dateSince: formatDateSince(deletionScheduledAt),
				})
			}}
			<RouterLink :to="{name: 'user.settings.deletion'}">
				{{ $t('user.deletion.scheduledCancel') }}
			</RouterLink>
		</Message>
		<div
			v-if="isLocalBuild"
			class="is-max-width-desktop overview-actions"
		>
			<PeriodReviewButton />
			<TasktraceTeamImportButton
				button-label="导入共享任务"
				@imported="updateTaskKey"
			/>
		</div>
		<ImportHint v-if="tasksLoaded" />
		<div
			v-if="authStore.settings.frontendSettings.showLastViewed !== false && projectHistory.length > 0"
			class="is-max-width-desktop has-text-start mbs-4"
		>
			<h2>{{ $t('home.lastViewed') }}</h2>
			<ProjectCardGrid
				v-cy="'projectCardGrid'"
				:projects="projectHistory"
				:show-even-number-of-projects="true"
			/>
		</div>
		<ShowTasks
			v-if="projectStore.hasProjects"
			:key="showTasksKey"
			:label-ids="labelIds"
			class="show-tasks"
			@tasksLoaded="tasksLoaded = true"
			@clearLabelFilter="handleClearLabelFilter"
		/>
	</div>
</template>

<script lang="ts" setup>
import {ref, computed} from 'vue'
import {useRoute, useRouter} from 'vue-router'

import Message from '@/components/misc/Message.vue'
import ShowTasks from '@/views/tasks/ShowTasks.vue'
import ProjectCardGrid from '@/components/project/partials/ProjectCardGrid.vue'
import TasktraceTeamImportButton from '@/components/project/TasktraceTeamImportButton.vue'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import PeriodReviewButton from '@/components/home/PeriodReviewButton.vue'
import ImportHint from '@/components/home/ImportHint.vue'

import {getHistory} from '@/modules/projectHistory'
import {parseDateOrNull} from '@/helpers/parseDateOrNull'
import {formatDateSince, formatDisplayDate} from '@/helpers/time/formatDate'
import {useDaytimeSalutation} from '@/composables/useDaytimeSalutation'

import {useProjectStore} from '@/stores/projects'
import {useAuthStore} from '@/stores/auth'

const salutation = useDaytimeSalutation()

const authStore = useAuthStore()
const projectStore = useProjectStore()
const route = useRoute()
const router = useRouter()

const projectHistory = computed(() => {
	// If we don't check this, it tries to load the project background right after logging out	
	if(!authStore.authenticated) {
		return []
	}
	
	return getHistory()
		.map(l => projectStore.projects[l.id])
		.filter(l => Boolean(l))
})

const tasksLoaded = ref(false)

const deletionScheduledAt = computed(() => parseDateOrNull(authStore.info?.deletionScheduledAt))

// Extract label IDs from query parameter
const labelIds = computed(() => {
	const labelsParam = route.query.labels
	if (!labelsParam) {
		return undefined
	}
	return Array.isArray(labelsParam) ? labelsParam : [labelsParam]
})

// Reload only after an import changes the tasks in the overview.
const showTasksKey = ref(0)

function updateTaskKey() {
	showTasksKey.value++
}

function handleClearLabelFilter() {
	const query = {...route.query}
	delete query.labels
	router.push({
		name: route.name as string,
		query,
	})
}
</script>

<style scoped lang="scss">
.overview-actions {
	display: flex;
	justify-content: flex-end;
	flex-wrap: wrap;
	gap: .75rem;
}
.show-tasks {
	margin-block-start: 2rem;
}
</style>
