<template>
	<p
		class="overview-drag-help"
		role="status"
	>
		{{ saving ? '正在保存任务所属关系…' : '按住任务名拖到同一项目的另一任务上，可移为其子任务；拖到项目标题上，可移为顶层任务。' }}
	</p>
	<section
		v-for="project in groups"
		:key="project.projectId"
		class="overview-project"
		:data-overview-project="project.projectId"
		:aria-label="projectTitle(project.projectId)"
	>
		<header
			class="overview-project-heading"
			:class="{'is-drop-target': rootDropProject === project.projectId}"
			@dragover.prevent="overProject($event, project.projectId)"
			@dragleave="leaveProject($event)"
			@drop.prevent="dropAtRoot(project.projectId)"
		>
			<button
				type="button"
				class="tree-toggle"
				:aria-expanded="!collapsedProjects.has(project.projectId)"
				:aria-label="`${collapsedProjects.has(project.projectId) ? '展开' : '收起'}项目 ${projectTitle(project.projectId)}`"
				@click="toggle(collapsedProjects, project.projectId)"
			>
				<Icon
					icon="chevron-down"
					:class="{'is-collapsed': collapsedProjects.has(project.projectId)}"
				/>
			</button>
			<h3>
				<RouterLink :to="{name: 'project.index', params: {projectId: project.projectId}}">
					{{ projectTitle(project.projectId) }}
				</RouterLink>
			</h3>
			<span>{{ project.count }} 项</span>
		</header>
		<ul
			v-if="!collapsedProjects.has(project.projectId)"
			class="overview-task-list"
		>
			<li
				v-for="row in visibleRows(project.rows)"
				:key="row.task.id"
				class="overview-task-row"
				:class="{'is-context': row.context}"
				:data-overview-task="row.task.id"
				:data-depth="row.depth"
				:style="{'--task-depth': Math.min(row.depth, 6)}"
			>
				<button
					v-if="row.hasChildren"
					type="button"
					class="tree-toggle"
					:aria-expanded="!collapsedTasks.has(row.task.id)"
					:aria-label="`${collapsedTasks.has(row.task.id) ? '展开' : '收起'}任务 ${row.task.title}`"
					@click="toggle(collapsedTasks, row.task.id)"
				>
					<Icon
						icon="chevron-down"
						:class="{'is-collapsed': collapsedTasks.has(row.task.id)}"
					/>
				</button>
				<span
					v-else
					class="tree-spacer"
					aria-hidden="true"
				/>
				<div class="overview-task-content">
					<SingleTaskInProject
						:the-task="row.task"
						:show-project="false"
						:title="row.context ? '为显示匹配的下级任务而保留的上级任务' : undefined"
						:can-mark-as-done="canWrite(row.task.projectId)"
						:can-drag="canWrite(row.task.projectId) && !saving && !loading"
						task-drop-mode="parent"
						@taskDragStart="startDrag"
						@taskDragEnd="endDrag"
						@taskDrop="dropOnTask"
						@taskUpdated="emit('taskUpdated', $event)"
					/>
				</div>
			</li>
		</ul>
	</section>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'
import Icon from '@/components/misc/Icon'
import SingleTaskInProject from './SingleTaskInProject.vue'
import {useProjectStore} from '@/stores/projects'
import {PERMISSIONS} from '@/constants/permissions'
import type {ITask} from '@/modelTypes/ITask'
import {groupOverviewTasks} from '@/helpers/overviewTasks'
import {taskSubtreeIds} from '@/helpers/taskTreeDrag'
import {tasksTasktraceMove} from '@/client/generated'
import {error, getErrorText, success} from '@/message'
const props = defineProps<{tasks: ITask[], ancestors: ITask[], loading?: boolean}>()
const emit = defineEmits<{taskUpdated: [task: ITask], taskMoved: []}>()
const projectStore = useProjectStore()
const groups = computed(() => groupOverviewTasks(props.tasks, props.ancestors))
const collapsedProjects = ref(new Set<number>())
const collapsedTasks = ref(new Set<number>())
const projectTitle = (id: number) => projectStore.projects[id]?.title || `项目 #${id}`
const draggedTask = ref<ITask | null>(null)
const rootDropProject = ref<number | null>(null)
const saving = ref(false)
const canWrite = (id: number) => !projectStore.projects[id]?.isArchived
	&& (projectStore.projects[id]?.maxPermission ?? 0) > PERMISSIONS.READ

function startDrag({task, event}: {task: ITask, event: DragEvent}) {
	if (saving.value || props.loading || !canWrite(task.projectId)) {
		event.preventDefault()
		return
	}
	draggedTask.value = task
}
function endDrag() {
	draggedTask.value = null
	rootDropProject.value = null
}
function overProject(event: DragEvent, projectId: number) {
	const allowed = draggedTask.value?.projectId === projectId && canWrite(projectId) && !saving.value
	rootDropProject.value = allowed ? projectId : null
	if (event.dataTransfer) event.dataTransfer.dropEffect = allowed ? 'move' : 'none'
}
function leaveProject(event: DragEvent) {
	if (event.relatedTarget instanceof Node && (event.currentTarget as HTMLElement).contains(event.relatedTarget)) return
	rootDropProject.value = null
}
async function dropAtRoot(projectId: number) {
	await moveTask(projectId, 0)
}
async function dropOnTask({task}: {task: ITask}) {
	await moveTask(task.projectId, task.id)
}
async function moveTask(projectId: number, parentId: number) {
	const source = draggedTask.value
	rootDropProject.value = null
	if (!source || saving.value || props.loading) return
	if (source.projectId !== projectId) {
		error({message: '请拖到同一项目中的任务或项目标题上。'})
		return
	}
	if (parentId && taskSubtreeIds(source.id, [...props.tasks, ...props.ancestors]).has(parentId)) {
		error({message: '不能将任务移到自身或自己的子任务下。'})
		return
	}
	const parents = source.relatedTasks?.parenttask || []
	if ((!parentId && !parents.length) || (parents.length === 1 && parents[0].id === parentId)) return
	const project = projectStore.projects[projectId]
	const view = project?.views.find(view => view.viewKind === 'list') || project?.views[0]
	if (!view) {
		error({message: '未找到项目视图，请刷新页面后重试。'})
		return
	}
	saving.value = true
	try {
		await tasksTasktraceMove({
			path: {task: source.id},
			body: {parent_id: parentId, before_task_id: 0, project_view_id: view.id},
			throwOnError: true,
		})
		if (parentId) collapsedTasks.value.delete(parentId)
		collapsedProjects.value.delete(projectId)
		emit('taskMoved')
		success({message: '任务及其全部子任务已移动，遗留事项保持在原任务中。'})
	} catch (cause) {
		error({message: '任务移动失败，请检查编辑权限以及任务层级是否超过 5 级。' + getErrorText(cause)})
	} finally {
		saving.value = false
		endDrag()
	}
}
function toggle(set: Set<number>, id: number) { if (set.has(id)) set.delete(id); else set.add(id) }
function visibleRows(rows: ReturnType<typeof groupOverviewTasks>[number]['rows']) {
	let hiddenBelow = Infinity
	return rows.filter(row => {
		if (row.depth > hiddenBelow) return false
		hiddenBelow = collapsedTasks.value.has(row.task.id) ? row.depth : Infinity
		return true
	})
}
</script>

<style scoped lang="scss">
.overview-drag-help {
	margin: 0;
	padding: .75rem;
	font-size: .875rem;
	color: var(--text);
}
.overview-project-heading.is-drop-target {
	outline: 2px solid var(--primary);
	outline-offset: 2px;
	background: var(--grey-100);
}
.overview-project {
	padding: .75rem;
}
.overview-project + .overview-project {
	border-block-start: 1px solid var(--grey-200);
}
.overview-project-heading {
	display: flex;
	align-items: center;
	gap: .5rem;
	h3 {
		margin: 0;
		font-size: 1rem;
		overflow-wrap: anywhere;
	}
	> span {
		margin-inline-start: auto;
		white-space: nowrap;
		font-size: .875rem;
	}
}
.overview-task-list {
	list-style: none;
	margin: .5rem 0 0;
	padding: 0;
}
.overview-task-row {
	display: flex;
	align-items: center;
	padding-inline-start: calc(var(--task-depth) * 1.25rem);
}
.tree-toggle, .tree-spacer {
	flex: 0 0 1.75rem;
	inline-size: 1.75rem;
	block-size: 2rem;
}
.tree-toggle {
	border: 0;
	background: transparent;
	color: var(--text);
	cursor: pointer;
	&:hover {
		background: var(--grey-100);
	}
	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: 2px;
	}
}
.overview-task-content {
	flex: 1;
	min-inline-size: 0;
}
.overview-task-row.is-context :deep(.single-task) { color: var(--grey-600); }
.is-collapsed { transform: rotate(-90deg); }
</style>
