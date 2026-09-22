<template>
	<ProjectWrapper
		class="project-list"
		:is-loading-project="isLoadingProject"
		:project-id="projectId"
		:view-id
	>
		<template #header>
			<div class="filter-container">
				<label v-if="projectId > 0">
					<input
						v-model="showCompleted"
						type="checkbox"
						aria-label="显示已完成任务"
					>
					显示已完成任务
				</label>
				<SortPopup
					v-model="sortByParam"
				/>
				<FilterPopup
					v-if="!isSavedFilter(project)"
					v-model="params"
					:view-id="viewId"
					:project-id="projectId"
				/>
			</div>
		</template>

		<template #default>
			<div
				:class="{ 'is-loading': loading }"
				class="loader-container is-max-width-desktop list-view"
			>
				<Card
					:padding="false"
					:has-content="false"
					class="has-overflow"
				>
					<AddTask
						v-if="!project?.isArchived && canWrite"
						ref="addTaskRef"
						class="list-view__add-task d-print-none"
						@tasksAdded="updateTaskList"
					/>

					<Nothing v-if="ctaVisible && tasks.length === 0 && !loading">
						{{ $t('project.list.empty') }}
						<ButtonLink
							v-if="project?.id > 0 && canWrite"
							@click="focusNewTaskInput()"
						>
							{{ $t('project.list.newTaskCta') }}
						</ButtonLink>
					</Nothing>

					<ul
						v-if="tasks && tasks.length > 0"
						class="tasks"
						:class="{'dragging-disabled': !canDragTasks || !isPositionSorting}"
						@dragover.self.prevent
						@drop.self.prevent="dropAtRootEnd"
					>
						<li
							v-for="(t, index) in tasks"
							:key="t.id"
							class="task-tree-root"
						>
							<SingleTaskInProject
								:ref="(el) => setTaskRef(el, index)"
								:show-list-color="false"
								:can-mark-as-done="canWrite || isPseudoProject"
								:the-task="t"
								:all-tasks="allTasks"
								:can-drag="canDragTasks && isPositionSorting && !dragMoveSaving"
								@taskUpdated="updateTasks"
								@taskDragStart="handleTaskDragStart"
								@taskDragEnd="handleTaskDragEnd"
								@taskDrop="handleTaskDrop"
							/>
						</li>
					</ul>

					<Pagination
						:total-pages="totalPages"
						:current-page="currentPage"
					/>
				</Card>
			</div>
		</template>
	</ProjectWrapper>
</template>


<script setup lang="ts">
import {ref, computed, nextTick, onMounted, onBeforeUnmount, watch, toRef} from 'vue'
import {useStorage} from '@vueuse/core'

import ProjectWrapper from '@/components/project/ProjectWrapper.vue'
import ButtonLink from '@/components/misc/ButtonLink.vue'
import AddTask from '@/components/tasks/AddTask.vue'
import SingleTaskInProject from '@/components/tasks/partials/SingleTaskInProject.vue'
import FilterPopup from '@/components/project/partials/FilterPopup.vue'
import Nothing from '@/components/misc/Nothing.vue'
import Pagination from '@/components/misc/Pagination.vue'
import SortPopup from '@/components/project/partials/SortPopup.vue'

import {useTaskList} from '@/composables/useTaskList'
import {useTaskDragToProject} from '@/composables/useTaskDragToProject'
import {shouldShowTaskInListView} from '@/composables/useTaskListFiltering'
import {PERMISSIONS as Permissions} from '@/constants/permissions'
import type {ITask} from '@/modelTypes/ITask'
import {taskMovePlan, type TaskDropZone} from '@/helpers/taskTreeDrag'
import {tasksTasktraceMove} from '@/client/generated'
import {error, success} from '@/message'
import {isSavedFilter, useSavedFilter} from '@/services/savedFilter'

import {useBaseStore} from '@/stores/base'
import {useTaskStore} from '@/stores/tasks'

import type {IProject} from '@/modelTypes/IProject'
import type {IProjectView} from '@/modelTypes/IProjectView'
const props = defineProps<{
        isLoadingProject: boolean,
        projectId: IProject['id'],
        viewId: IProjectView['id'],
}>()

const projectId = toRef(props, 'projectId')

defineOptions({name: 'List'})

const showCompleted = useStorage('tasktrace:edit-show-completed', false)
const ctaVisible = ref(false)

const {
	tasks: allTasks,
	loading,
	totalPages,
	currentPage,
	loadTasks,
	params,
	sortByParam,
} = useTaskList(
	() => projectId.value,
	() => props.viewId,
	{position: 'asc'},
	() => projectId.value === -1
		? ['comment_count', 'is_unread']
		: ['subtasks', 'comment_count', 'is_unread'],
	() => showCompleted.value,
)

// Saved filter composable for accessing filter data
const _savedFilter = useSavedFilter(() => isSavedFilter({id: projectId.value}) ? projectId.value : undefined).filter

const tasks = ref<ITask[]>([])
watch(
	allTasks,
	() => {
		tasks.value = ([...allTasks.value]).filter(t => shouldShowTaskInListView(t, allTasks.value))
	},
)

const isPositionSorting = computed(() => !showCompleted.value && 'position' in sortByParam.value)

const baseStore = useBaseStore()
const taskStore = useTaskStore()
const {handleTaskDropToProject} = useTaskDragToProject()
const project = computed(() => baseStore.currentProject)

const canWrite = computed(() => {
	return project.value?.maxPermission > Permissions.READ && project.value?.id > 0
})

const isPseudoProject = computed(() => (project.value && isSavedFilter(project.value)) || project.value?.id === -1)

onMounted(async () => {
	await nextTick()
	ctaVisible.value = true
})

const canDragTasks = computed(() => canWrite.value || isSavedFilter(project.value))

const addTaskRef = ref<typeof AddTask | null>(null)

function focusNewTaskInput() {
	addTaskRef.value?.focusTaskInput()
}

function updateTaskList(newTasks: ITask[]) {
	if (!isPositionSorting.value) {
		// reload tasks with current filter and sorting
		loadTasks()
	} else {
		allTasks.value = [
			...newTasks,
			...allTasks.value,
		]
	}

	baseStore.setHasTasks(true)
}

function updateTasks(updatedTask: ITask) {
	if (projectId.value < 0) {
		// Reload tasks to keep saved filter results in sync
		loadTasks(false)
		return
	}

	for (let t = 0; t < tasks.value.length; t++) {
		if (tasks.value[t].id === updatedTask.id) {
			tasks.value[t] = updatedTask
			break
		}
	}
}

interface TaskDragPayload {
	task: ITask
	event: DragEvent
}

interface TaskDropPayload extends TaskDragPayload {
	zone: TaskDropZone
}

const draggedTaskId = ref(0)
const dragMoveSaving = ref(false)
let treeDropHandled = false

function handleTaskDragStart({task}: TaskDragPayload) {
	draggedTaskId.value = task.id
	treeDropHandled = false
	taskStore.setDraggedTask(task)
}

async function saveTreeMove(parentId: number, beforeTaskId: number) {
	if (!draggedTaskId.value || dragMoveSaving.value) return
	dragMoveSaving.value = true
	try {
		await tasksTasktraceMove({
			path: {task: draggedTaskId.value},
			body: {
				parent_id: parentId,
				before_task_id: beforeTaskId,
				project_view_id: props.viewId,
			},
		})
		await loadTasks(false)
		success({message: '任务及其全部子任务已移动，遗留事项保持在原任务中。'})
	} catch {
		error({message: '任务移动失败。请确认没有形成循环，并且任务层级不超过 5 级。'})
		await loadTasks(false)
	} finally {
		dragMoveSaving.value = false
	}
}

async function handleTaskDrop({task, zone}: TaskDropPayload) {
	if (!draggedTaskId.value) return
	treeDropHandled = true
	const plan = taskMovePlan(allTasks.value, draggedTaskId.value, task.id, zone)
	if (!plan) {
		error({message: '不能把任务移动到自身或自己的子任务中。'})
		return
	}
	await saveTreeMove(plan.parentId, plan.beforeTaskId)
}

async function dropAtRootEnd() {
	if (!draggedTaskId.value) return
	treeDropHandled = true
	await saveTreeMove(0, 0)
}

async function handleTaskDragEnd({event}: TaskDragPayload) {
	if (!treeDropHandled) {
		await handleTaskDropToProject({originalEvent: event}, (task) => {
			tasks.value = tasks.value.filter(item => item.id !== task.id)
		})
	}
	taskStore.setDraggedTask(null)
	draggedTaskId.value = 0
	treeDropHandled = false
}

const taskRefs = ref<(InstanceType<typeof SingleTaskInProject> | null)[]>([])
const focusedIndex = ref(-1)

function setTaskRef(el: InstanceType<typeof SingleTaskInProject> | null, index: number) {
	if (el === null) {
		delete taskRefs.value[index]
	} else {
		taskRefs.value[index] = el
	}
}

function focusTask(index: number) {
	if (index < 0 || index >= tasks.value.length) {
		return
	}

	const taskRef = taskRefs.value[index]

	focusedIndex.value = index
	taskRef?.focus()
}

function handleListNavigation(e: KeyboardEvent) {
	if (e.target instanceof HTMLElement && (e.target.closest('input, textarea, select, [contenteditable="true"]'))) {
		return
	}

	if (e.code === 'KeyJ') {
		e.preventDefault()
		focusTask(Math.min(focusedIndex.value + 1, tasks.value.length - 1))
		return
	}

	if (e.code === 'KeyK') {
		e.preventDefault()
		if (focusedIndex.value === -1) {
			focusTask(tasks.value.length - 1)
			return
		}

		if (focusedIndex.value === 0) {
			addTaskRef.value?.focusTaskInput()
			focusedIndex.value = -1
			return
		}

		focusTask(Math.max(focusedIndex.value - 1, 0))
		return
	}

	if (e.code === 'Enter') {
		if (e.isComposing) {
			return
		}

		// Links and buttons activate natively on Enter; leave them alone
		if (e.target instanceof HTMLElement && e.target.closest('a, button, [role="button"]')) {
			return
		}

		// Only act when a row was focused via J/K roving navigation
		if (focusedIndex.value < 0) {
			return
		}

		e.preventDefault()
		taskRefs.value[focusedIndex.value]?.click(e)
	}
}

onMounted(() => {
	document.addEventListener('keydown', handleListNavigation)
})

onBeforeUnmount(() => {
	document.removeEventListener('keydown', handleListNavigation)
})
</script>

<style lang="scss" scoped>
.filter-container {
	display: flex;
	align-items: center;
	gap: .5rem;

	:deep(.popup) {
		max-inline-size: 300px;
	}
}

.tasks {
	padding: .5rem;
}

.list-view__add-task {
	padding: 1rem 1rem 0;
}

.link-share-view .card {
	border: none;
	box-shadow: none;
}

.task-tree-root {
	list-style: none;
}

.list-view {
	padding-block-end: 1rem;

	:deep(.card) {
		margin-block-end: 0;
	}
}
</style>
