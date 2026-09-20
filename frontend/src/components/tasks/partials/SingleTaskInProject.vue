<template>
	<div
		:data-task-id="task.id"
		:data-project-id="task.projectId"
		class="task-tree-item"
	>
		<div
			ref="taskRoot"
			:class="{
				'is-loading': taskService.loading,
				'task-drop-before': dropZone === 'before',
				'task-drop-inside': dropZone === 'inside',
				'task-drop-after': dropZone === 'after',
			}"
			class="task loader-container single-task"
			tabindex="-1"
			:data-is-overdue="isOverdue || undefined"
			@dragenter.prevent.stop
			@dragover.prevent.stop="showTaskDropZone"
			@dragleave.stop="clearTaskDropZone"
			@drop.prevent.stop="dropOnTask"
		>
			<span
				v-tooltip="!canMarkAsDone ? $t('task.readOnlyCheckbox') : ''"
				class="is-inline-flex is-align-items-center"
			>
				<FancyCheckbox
					v-model="task.done"
					:disabled="isArchived || disabled || !canMarkAsDone"
					:aria-label="$t('task.detail.markAsDone', {task: task.title})"
					@update:modelValue="markAsDone"
					@click.stop
				/>
			</span>

			<ColorBubble
				v-if="!showProjectSeparately && projectColor !== '' && currentProject?.id !== task.projectId"
				:color="projectColor"
				class="mie-1"
			/>

			<div
				:class="{ 'done': task.done, 'show-project': showProject && project}"
				class="tasktext"
			>
				<span>
					<RouterLink
						v-if="showProject && typeof project !== 'undefined'"
						v-tooltip="$t('task.detail.belongsToProject', {project: project.title})"
						:to="{ name: 'project.index', params: { projectId: task.projectId } }"
						class="task-project mie-1"
						:class="{'mie-2': task.hexColor !== ''}"
						@click.stop
					>
						{{ project.title }}
					</RouterLink>

					<ColorBubble
						v-if="task.hexColor !== ''"
						:color="getHexColor(task.hexColor)"
						class="mie-1"
					/>
	
					<label
						class="inline-task-control mie-1"
						@click.stop
						@pointerdown.stop
					>
						<span class="is-sr-only">任务优先级</span>
						<select
							:value="tasktracePriorityNumber(task.priority)"
							:disabled="inlineSaving || disabled || isArchived"
							aria-label="任务优先级"
							@change="changePriority"
						>
							<option
								v-for="priority in 10"
								:key="priority - 1"
								:value="priority - 1"
							>P{{ priority - 1 }}</option>
						</select>
					</label>

					<label
						class="inline-task-control mie-1"
						@click.stop
						@pointerdown.stop
					>
						<span class="is-sr-only">任务状态</span>
						<select
							:value="task.status"
							:disabled="inlineSaving || disabled || isArchived"
							aria-label="任务状态"
							@change="changeStatus"
						>
							<option
								v-for="option in TASK_STATUS_OPTIONS"
								:key="option.value"
								:value="option.value"
							>{{ option.label }}</option>
						</select>
					</label>

					<TaskGlanceTooltip :task="task">
						<RouterLink
							:to="taskDetailRoute"
							:draggable="canDrag"
							class="task-link"
							title="单击编辑；按住并拖动可改变层级"
							@dragstart.stop="startTaskDrag"
							@dragend.stop="finishTaskDrag"
						>
							{{ task.title }}
						</RouterLink>
					</TaskGlanceTooltip>
				</span>

				<Labels
					v-if="task.labels.length > 0"
					class="labels mis-2 mie-1"
					:labels="task.labels"
				/>

				<AssigneeList
					v-if="task.assignees.length > 0"
					:assignees="task.assignees"
					:avatar-size="25"
					class="mis-1"
					:inline="true"
				/>

				<Popup
					v-if="+new Date(task.dueDate) > 0"
					placement="bottom-start"
					:anchor="dueDateTriggerEl"
					sheet-on-mobile
					:sheet-title="$t('task.deferDueDate.title')"
				>
					<template #trigger="{toggle, isOpen}">
						<BaseButton
							ref="dueDateTrigger"
							v-tooltip="formatDateLong(task.dueDate)"
							class="dueDate"
							@click.prevent.stop="toggle()"
						>	
							<time
								:datetime="formatISO(task.dueDate)"
								class="is-italic"
								:aria-expanded="isOpen ? 'true' : 'false'"
							>
								– {{ $t('task.detail.due', {at: dueDateFormatted}) }}
							</time>
						</BaseButton>
					</template>
					<template #content="{isOpen}">
						<DeferTask
							v-if="isOpen"
							v-model="task"
						/>
					</template>
				</Popup>

				<span>
					<span
						v-if="task.attachments.length > 0"
						class="project-task-icon"
						role="img"
						:aria-label="$t('task.attributes.attachment', task.attachments.length)"
					>
						<Icon icon="paperclip" />
					</span>
					<span
						v-if="!isEditorContentEmpty(task.description)"
						class="project-task-icon is-mirrored-rtl"
					>
						<Icon icon="align-left" />
					</span>
					<span
						v-if="isRepeating"
						class="project-task-icon"
					>
						<Icon icon="history" />
					</span>
					<CommentCount
						:task="task"
						class="project-task-icon"
					/>
				</span>

				<ChecklistSummary :task="task" />
			</div>

			<ProgressBar
				v-if="task.percentDone > 0"
				:value="task.percentDone * 100"
				is-small
			/>

			<ColorBubble
				v-if="showProjectSeparately && projectColor !== '' && currentProject?.id !== task.projectId"
				:color="projectColor"
				class="mie-1"
			/>

			<RouterLink
				v-if="showProjectSeparately"
				v-tooltip="$t('task.detail.belongsToProject', {project: project.title})"
				:to="{ name: 'project.index', params: { projectId: task.projectId } }"
				class="task-project"
				@click.stop
			>
				{{ project.title }}
			</RouterLink>

			<BaseButton
				:class="{'is-favorite': task.isFavorite}"
				class="favorite"
				@click.stop="toggleFavorite"
			>
				<span class="is-sr-only">{{ task.isFavorite ? $t('task.detail.actions.unfavorite') : $t('task.detail.actions.favorite') }}</span>
				<Icon
					v-if="task.isFavorite"
					icon="star"
				/>
				<Icon
					v-else
					:icon="['far', 'star']"
				/>
			</BaseButton>
			<slot />
		</div>
		<single-task-in-project
			v-for="subtask in orderedSubtasks"
			:key="subtask.id"
			:the-task="subtask"
			:disabled="disabled"
			:can-mark-as-done="canMarkAsDone"
			:all-tasks="allTasks"
			:can-drag="canDrag"
			class="subtask-nested"
			@taskUpdated="emit('taskUpdated', $event)"
			@taskDragStart="emit('taskDragStart', $event)"
			@taskDragEnd="emit('taskDragEnd', $event)"
			@taskDrop="emit('taskDrop', $event)"
		/>
	</div>
</template>

<script setup lang="ts">
import {ref, watch, shallowReactive, onMounted, computed} from 'vue'
import {useI18n} from 'vue-i18n'
import {useRouter} from 'vue-router'

import TaskModel, {getHexColor} from '@/models/task'
import type {ITask} from '@/modelTypes/ITask'

import Labels from '@/components/tasks/partials/Labels.vue'
import TaskGlanceTooltip from '@/components/tasks/partials/TaskGlanceTooltip.vue'
import DeferTask from '@/components/tasks/partials/DeferTask.vue'
import ChecklistSummary from '@/components/tasks/partials/ChecklistSummary.vue'
import CommentCount from '@/components/tasks/partials/CommentCount.vue'

import ProgressBar from '@/components/misc/ProgressBar.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import FancyCheckbox from '@/components/input/FancyCheckbox.vue'
import ColorBubble from '@/components/misc/ColorBubble.vue'
import Popup from '@/components/misc/Popup.vue'

import TaskService from '@/services/task'

import {formatDisplayDate, formatISO, formatDateLong} from '@/helpers/time/formatDate'
import {error, success} from '@/message'

import {useProjectStore} from '@/stores/projects'
import {useBaseStore} from '@/stores/base'
import {useTaskStore} from '@/stores/tasks'
import AssigneeList from '@/components/tasks/partials/AssigneeList.vue'
import {useIntervalFn} from '@vueuse/core'
import {playPopSound} from '@/helpers/playPop'
import {isEditorContentEmpty} from '@/helpers/editorContentEmpty'
import {TASK_REPEAT_MODES} from '@/types/IRepeatMode'
import {TASK_STATUSES, TASK_STATUS_OPTIONS, type TaskStatus} from '@/types/ITaskStatus'
import {useGlobalNow} from '@/composables/useGlobalNow'
import type {TaskDropZone} from '@/helpers/taskTreeDrag'
import {tasktracePriorityNumber, tasktraceStoredPriority} from '@/helpers/tasktracePriority'
import type {Priority} from '@/constants/priorities'

interface TaskDragEvent {
	task: ITask
	event: DragEvent
}

interface TaskDropEvent extends TaskDragEvent {
	zone: TaskDropZone
}

const props = withDefaults(defineProps<{
	theTask: ITask,
	isArchived?: boolean,
	showProject?: boolean,
	disabled?: boolean,
	canMarkAsDone?: boolean,
	allTasks?: ITask[],
	canDrag?: boolean,
}>(), {
	isArchived: false,
	showProject: false,
	disabled: false,
	canMarkAsDone: true,
	allTasks: () => [],
	canDrag: false,
})

const emit = defineEmits<{
	'taskUpdated': [task: ITask],
	'taskDragStart': [payload: TaskDragEvent],
	'taskDragEnd': [payload: TaskDragEvent],
	'taskDrop': [payload: TaskDropEvent],
}>()

function getTaskById(taskId: number): ITask | undefined {
	if (typeof props.allTasks === 'undefined' || props.allTasks.length === 0) {
		return undefined
	}

	return props.allTasks.find(t => t.id === taskId)
}

const {t} = useI18n({useScope: 'global'})
const router = useRouter()

const taskService = shallowReactive(new TaskService())
const task = ref<ITask>(new TaskModel())

const orderedSubtasks = computed(() => {
	const order = new Map(props.allTasks.map((item, index) => [item.id, index]))
	return (task.value.relatedTasks?.subtask ?? [])
		.map(subtask => getTaskById(subtask.id))
		.filter((subtask): subtask is ITask => typeof subtask !== 'undefined')
		.sort((left, right) => (order.get(left.id) ?? Number.MAX_SAFE_INTEGER) - (order.get(right.id) ?? Number.MAX_SAFE_INTEGER))
})

const isRepeating = computed(() => task.value.repeatAfter.amount > 0 || (task.value.repeatAfter.amount === 0 && task.value.repeatMode === TASK_REPEAT_MODES.REPEAT_MODE_MONTH))

watch(
	() => props.theTask,
	newVal => {
		task.value = newVal
	},
	{
		immediate: true,
		deep: true,
	},
)

const baseStore = useBaseStore()
const projectStore = useProjectStore()
const taskStore = useTaskStore()

const project = computed(() => projectStore.projects[task.value.projectId])
const projectColor = computed(() => project.value ? project.value?.hexColor : '')

const showProjectSeparately = computed(() => !props.showProject && currentProject.value?.id !== task.value.projectId && project.value)

const currentProject = computed(() => {
	return typeof baseStore.currentProject === 'undefined' ? {
		id: 0,
		title: '',
	} : baseStore.currentProject
})

const taskDetailRoute = computed(() => ({
	name: 'task.detail',
	params: {id: task.value.id},
	// TODO: re-enable opening task detail in modal
	// state: { backdropView: router.currentRoute.value.fullPath },
}))

function updateDueDate() {
	if (!task.value.dueDate) {
		return
	}

	dueDateFormatted.value = formatDisplayDate(task.value.dueDate)
}

const dueDateFormatted = ref('')
useIntervalFn(updateDueDate, 60_000, {
	immediateCallback: true,
})
onMounted(updateDueDate)

watch(() => task.value.dueDate, updateDueDate)

const {now} = useGlobalNow()
const isOverdue = computed(() => (
	!task.value.done &&
	task.value.dueDate !== null &&
	task.value.dueDate.getTime() > 0 &&
	task.value.dueDate.getTime() <= now.value.getTime()
))

let oldTask

async function markAsDone(checked: boolean, wasReverted: boolean = false) {
	oldTask = {...task.value}

	// Fire the request immediately and with the intended done value snapshotted, so a re-render or
	// teardown during the animation delay can neither drop the save nor make it send a stale state.
	const updatePromise = taskStore.update({
		...task.value,
		done: checked,
		status: checked ? TASK_STATUSES.DONE : TASK_STATUSES.TODO,
	})

	const finish = async () => {
		const newTask = await updatePromise
		task.value = newTask

		updateDueDate()

		if (wasReverted) {
			return
		}

		if (checked) {
			playPopSound()
		}
		emit('taskUpdated', newTask)

		let message = t('task.doneSuccess')
		if (!task.value.done && !isRepeating.value) {
			message = t('task.undoneSuccess')
		}

		success({message}, [{
			title: t('task.undo'),
			callback: () => undoDone(checked),
		}])
	}

	if (checked) {
		setTimeout(finish, 300) // Delay only the follow-up to show the animation when marking a task as done
	} else {
		await finish() // Don't delay it when un-marking it as it doesn't have an animation the other way around
	}
}

function undoDone(checked: boolean) {
	if (isRepeating.value) {
		task.value = {...oldTask}
	}
	task.value.done = !task.value.done
	markAsDone(!checked, true)
}

async function toggleFavorite() {
	task.value = await taskStore.toggleFavorite(task.value)
	emit('taskUpdated', task.value)
}

const taskRoot = ref<HTMLElement | null>(null)
const dueDateTrigger = ref<InstanceType<typeof BaseButton> | null>(null)
const dueDateTriggerEl = computed<HTMLElement | null>(() => dueDateTrigger.value?.$el ?? null)
const inlineSaving = ref(false)

async function saveInline(changes: Partial<ITask>) {
	if (inlineSaving.value || props.disabled || props.isArchived) return
	inlineSaving.value = true
	try {
		const updated = await taskStore.update({...task.value, ...changes})
		task.value = updated
		emit('taskUpdated', updated)
	} catch (reason) {
		error(reason)
	} finally {
		inlineSaving.value = false
	}
}

function changePriority(event: Event) {
	const displayed = Number((event.target as HTMLSelectElement).value)
	void saveInline({priority: tasktraceStoredPriority(displayed) as Priority})
}

function changeStatus(event: Event) {
	const status = (event.target as HTMLSelectElement).value as TaskStatus
	void saveInline({status, done: status === TASK_STATUSES.DONE})
}

const dropZone = ref<TaskDropZone | null>(null)

function startTaskDrag(event: DragEvent) {
	if (!props.canDrag) {
		event.preventDefault()
		return
	}
	if (event.dataTransfer) {
		event.dataTransfer.effectAllowed = 'move'
		event.dataTransfer.setData('text/plain', String(task.value.id))
	}
	emit('taskDragStart', {task: task.value, event})
}

function finishTaskDrag(event: DragEvent) {
	dropZone.value = null
	emit('taskDragEnd', {task: task.value, event})
}

function zoneForEvent(event: DragEvent): TaskDropZone {
	const bounds = taskRoot.value?.getBoundingClientRect()
	if (!bounds) return 'inside'
	const offset = event.clientY - bounds.top
	if (offset < bounds.height * .25) return 'before'
	if (offset > bounds.height * .75) return 'after'
	return 'inside'
}

function showTaskDropZone(event: DragEvent) {
	if (!props.canDrag) return
	dropZone.value = zoneForEvent(event)
	if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
}

function clearTaskDropZone(event: DragEvent) {
	if (event.relatedTarget instanceof Node && taskRoot.value?.contains(event.relatedTarget)) return
	dropZone.value = null
}

function dropOnTask(event: DragEvent) {
	if (!props.canDrag) return
	const zone = zoneForEvent(event)
	dropZone.value = null
	emit('taskDrop', {task: task.value, event, zone})
}

defineExpose({
	focus: () => taskRoot.value?.focus(),
	click: () => router.push(taskDetailRoute.value),
})
</script>

<style lang="scss" scoped>
.task {
	display: flex;
	flex-wrap: wrap;
	padding: .4rem;
	transition: background-color $transition;
	align-items: center;
	cursor: default;
	border-radius: $radius;
	border: 2px solid transparent;

	&:hover {
		background-color: var(--grey-100);
	}

	&:has(*:focus-visible), &:focus {
		box-shadow: 0 0 0 2px hsla(var(--primary-hsl), 0.5);

		a.task-link {
			box-shadow: none;
		}
	}

	@supports not selector(:focus-within) {
		:focus {
			box-shadow: 0 0 0 2px hsla(var(--primary-hsl), 0.5);

			a.task-link {
				box-shadow: none;
			}
		}
	}

	.tasktext,
	&.tasktext {
		text-overflow: ellipsis;
		word-wrap: break-word;
		word-break: break-word;
		display: -webkit-box;
		hyphens: auto;
		-webkit-line-clamp: 4;
		-webkit-box-orient: vertical;
		overflow: hidden;

		flex: 1 0 50%;

	}

	.dueDate {
		display: inline-block;
		margin-inline-start: 5px;

		&:focus-visible {
			box-shadow: none;

			time {
				box-shadow: 0 0 0 1px hsla(var(--primary-hsl), 0.5);
				border-radius: 3px;
			}
		}
	}

	&[data-is-overdue] .dueDate {
		color: var(--danger-text);
	}

	.task-project {
		inline-size: auto;
		color: var(--grey-400);
		font-size: .9rem;
		white-space: nowrap;
	}

	.tasktext :deep(.color-bubble),
	.tasktext :deep(.avatar-wrapper),
	.tasktext :deep(.labels .tag) {
		vertical-align: middle;
		transform: translateY(-2px);
	}

	.avatar {
		border-radius: 50%;
		vertical-align: bottom;
		margin-inline-start: 5px;
		block-size: 27px;
		inline-size: 27px;
	}

	.project-task-icon {
		margin-inline-start: 6px;

		&:not(:first-of-type) {
			margin-inline-start: 8px;
		}

	}

	a {
		color: var(--text);
		transition: color ease $transition-duration;

		&:hover {
			color: var(--grey-900);
		}
	}

	.favorite {
		opacity: 1;
		text-align: center;
		inline-size: 27px;
		transition: opacity $transition, color $transition;
		border-radius: $radius;

		&:hover {
			color: var(--warning);
		}

		&.is-favorite {
			opacity: 1;
			color: var(--warning);
		}
	}

	@media(hover: hover) and (pointer: fine) {
		& .favorite {
			opacity: 0;
		}

		&:hover .favorite {
			opacity: 1;
		}
	}

	.favorite:focus {
		opacity: 1;
	}

	:deep(.fancy-checkbox) {
		block-size: 18px;
		padding-block-start: 0;
		padding-inline-end: .5rem;

		span {
			display: none;
		}

		// Extend the hit target to >=44x44 without affecting layout (WCAG 2.5.5).
		.base-checkbox__label {
			position: relative;

			&::before {
				content: '';
				position: absolute;
				inset-block-start: 50%;
				inset-inline-start: 50%;
				min-block-size: 44px;
				min-inline-size: 44px;
				block-size: 100%;
				inline-size: 100%;
				transform: translate(-50%, -50%);
			}
		}
	}

	.tasktext.done {
		text-decoration: line-through;
		color: var(--grey-500);
	}

	span.parent-tasks {
		color: var(--grey-500);
		inline-size: auto;
	}

	.show-project .parent-tasks {
		padding-inline-start: .25rem;
	}

	.remove {
		color: var(--danger);
	}

	input[type='checkbox'] {
		vertical-align: middle;
	}

	.settings {
		float: inline-end;
		inline-size: 24px;
		cursor: pointer;
	}

	&.loader-container.is-loading:after {
		inset-block-start: calc(50% - 1rem);
		inset-inline-start: calc(50% - 1rem);
		inline-size: 2rem;
		block-size: 2rem;
		border-inline-start-color: var(--grey-300);
		border-block-end-color: var(--grey-300);
	}
}

.inline-task-control select {
	min-block-size: 1.75rem;
	border: 1px solid var(--grey-300);
	border-radius: .25rem;
	background: var(--white);
	color: var(--text);
	font: inherit;
	cursor: pointer;
	padding: .1rem 1.45rem .1rem .35rem;
}

.task-link[draggable='true'] {
	cursor: grab;
	user-select: none;

	&:active {
		cursor: grabbing;
	}
}

.task-drop-before {
	box-shadow: inset 0 3px 0 var(--primary);
}

.task-drop-inside {
	border-color: var(--primary);
	background: hsla(var(--primary-hsl), .08);
}

.task-drop-after {
	box-shadow: inset 0 -3px 0 var(--primary);
}

.subtask-nested {
	margin-inline-start: 1.75rem;
}

:deep(.popup) {
	border-radius: $radius;
	background-color: var(--white);
	box-shadow: var(--shadow-lg);
	color: var(--text);

	&.is-open {
		padding: 1rem;
		border: 1px solid var(--grey-200);
	}
}
</style>

<style scoped lang="scss">
.task-status-badge {
	display: inline-flex;
	align-items: center;
	padding: .1rem .45rem;
	border-radius: 999px;
	background: var(--grey-200);
	color: var(--grey-700);
	font-size: .75rem;
	font-weight: 600;
	white-space: nowrap;
	&[data-status="doing"] {
		background: hsl(210deg 85% 93%);
		color: hsl(214deg 75% 35%);
	}

	&[data-status="hold"] {
		background: hsl(42deg 90% 90%);
		color: hsl(34deg 75% 30%);
	}

	&[data-status="done"] {
		background: hsl(145deg 55% 90%);
		color: hsl(145deg 55% 28%);
	}
}
</style>
