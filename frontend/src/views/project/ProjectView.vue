<script setup lang="ts">
import {computed, ref, shallowReactive, watch, watchEffect} from 'vue'
import {useRoute, useRouter} from 'vue-router'

import {useBaseStore} from '@/stores/base'
import {useProjectStore} from '@/stores/projects'
import {useAuthStore} from '@/stores/auth'

import {saveProjectView} from '@/helpers/projectView'
import ProjectService from '@/services/project'

import ProjectProgressOverview from '@/components/project/views/ProjectProgressOverview.vue'
import ProjectList from '@/components/project/views/ProjectList.vue'
import ProjectGantt from '@/components/project/views/ProjectGantt.vue'
import ProjectTable from '@/components/project/views/ProjectTable.vue'
import ProjectKanban from '@/components/project/views/ProjectKanban.vue'

import {DEFAULT_PROJECT_VIEW_SETTINGS} from '@/modelTypes/IProjectView'
import {saveProjectToHistory} from '@/modules/projectHistory'

const props = defineProps<{
	projectId: number,
	viewId: number,
}>()

const router = useRouter()
const baseStore = useBaseStore()
const projectStore = useProjectStore()
const authStore = useAuthStore()
const route = useRoute()

const editing = computed(() => props.projectId <= 0 || route.query.mode === 'edit')
function setEditing(value: boolean) { void router.replace({query: {...route.query, mode: value ? 'edit' : 'browse'}}) }
const currentProject = computed(() => projectStore.projects[props.projectId])

const currentView = computed(() => {
	return currentProject.value?.views.find(v => v.id === props.viewId)
})

const projectService = shallowReactive(new ProjectService())
const isLoadingProject = computed(() => projectService.loading)
const loadedProjectId = ref(0)

watch(
	() => props.projectId,
	// loadProject
	async (projectIdToLoad, oldProjectIdToLoad) => {

		console.debug('Loading project, $route.params =', route.params, `, loadedProjectId = ${loadedProjectId.value}, currentProject = `, currentProject.value)


		if (projectIdToLoad !== oldProjectIdToLoad) {
			loadedProjectId.value = 0
		}

		try {
			const loadedProject = await projectService.get({id: projectIdToLoad})

			// Here, we only set the new project in the projectStore.
			// Setting that projet as the current one in the baseStore is handled by the watcher below.
			projectStore.setProject(loadedProject)
		} finally {
			loadedProjectId.value = projectIdToLoad
		}
	},
	{immediate: true},
)

watch(
	() => [currentProject.value, props.viewId],
	([newCurrentProject, newViewId]) => {
		if (!newCurrentProject) {
			baseStore.handleSetCurrentProject({project: null})
			return
		}
		
		baseStore.handleSetCurrentProject({
			project: newCurrentProject,
			currentProjectViewId: newViewId,
		})
	}, {
		deep: true,
		immediate: true,
	},
)

function redirectToDefaultViewIfNecessary() {
	if (props.viewId === 0 || !currentView.value) {
		// Ideally, we would do that in the router redirect, but the projects (and therefore, the views) 
		// are not always loaded then.

		const defaultView  = authStore.settings.frontendSettings.defaultView

		let view
		if (defaultView !== DEFAULT_PROJECT_VIEW_SETTINGS.FIRST) {
			view = currentProject.value?.views.find(v => v.viewKind === defaultView)
		}

		// Use the first view as fallback if the default view is not available
		if (view === undefined && currentProject.value?.views?.length > 0) {
			view = currentProject.value?.views[0]
		}

		if (view) {
			router.replace({
				name: 'project.view',
				query: route.query,
				params: {
					projectId: props.projectId,
					viewId: view.id,
				},
			})
		}
	}
}

watch(
	() => props.viewId,
	redirectToDefaultViewIfNecessary,
	{immediate: true},
)

watch(
	currentProject,
	redirectToDefaultViewIfNecessary,
)

watchEffect(() => {
	// Don't save to history if the user is not authenticated (e.g., during logout)
	if (authStore.authenticated) {
		saveProjectToHistory({id: props.projectId})
	}
})
watchEffect(() => saveProjectView(props.projectId, props.viewId))

watchEffect(() => baseStore.setCurrentProjectViewId(props.viewId))
</script>

<template>
	<div
		v-if="projectId > 0"
		class="project-mode-bar"
	>
		<div><strong>{{ editing ? '编辑模式' : '展示模式' }}</strong><span>{{ editing ? '完成修改后返回展示模式查看整体进展' : '浏览项目进展，展开查看，不会修改内容' }}</span></div>
		<XButton
			v-if="editing || (currentProject?.maxPermission > 0 && !currentProject?.isArchived)"
			class="button is-primary"
			@click="setEditing(!editing)"
		>
			{{ editing ? '返回展示模式' : '进入编辑模式' }}
		</XButton>
	</div>
	<ProjectProgressOverview
		v-if="!editing && !isLoadingProject && loadedProjectId === projectId"
		:key="projectId"
		:project-id="projectId"
	/>
	<template v-if="editing">
		<ProjectList
			v-if="currentView?.viewKind === 'list'"
			:project-id="projectId"
			:is-loading-project="isLoadingProject"
			:view-id
		/>
		<ProjectGantt
			v-if="currentView?.viewKind === 'gantt'"
			:project-id="projectId"
			:route
			:is-loading-project="isLoadingProject"
			:view-id
		/>
		<ProjectTable
			v-if="currentView?.viewKind === 'table'"
			:project-id="projectId"
			:is-loading-project="isLoadingProject"
			:view-id
		/>
		<ProjectKanban
			v-if="currentView?.viewKind === 'kanban'"
			:project-id="projectId"
			:is-loading-project="isLoadingProject"
			:view-id
		/>
	</template>
</template>

<style scoped lang="scss">
.project-mode-bar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    flex-wrap: wrap;
    margin-block-end: 1rem;
    span {
	display: block;
	font-size: .8125rem;
	color: var(--grey-600);
	margin-block-start: .2rem;
	}
}
</style>