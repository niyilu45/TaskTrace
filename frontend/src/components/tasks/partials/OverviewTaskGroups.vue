<template>
	<p
		class="overview-drag-help"
		role="status"
	>
		{{ saving ? '正在保存任务所属关系…' : '按住任务名拖动，可用滚轮滚动页面；放到另一任务下成为子任务，放到项目的虚线框成为顶层任务。' }}
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
		<div
			v-if="canWrite(project.projectId)"
			class="overview-project-root-drop"
			:class="{'is-drop-target': rootDropProject === project.projectId}"
		>
			拖到这里，放在“{{ projectTitle(project.projectId) }}”下作为顶层任务
		</div>
		<ul
			v-if="!collapsedProjects.has(project.projectId)"
			class="overview-task-list"
		>
			<li
				v-for="row in visibleRows(project.rows)"
				:key="row.task.id"
				class="overview-task-row"
				:class="{'is-context': row.context, 'is-task-drop-target': taskDropTarget === row.task.id, 'is-drag-source': draggedTask?.id === row.task.id}"
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
						:pointer-drag="true"
						@taskPointerDown="({task, event}) => beginPointerDrag(task, event)"
						@taskUpdated="emit('taskUpdated', $event)"
					/>
				</div>
			</li>
		</ul>
	</section>
	<Teleport to="body">
		<div
			v-if="draggedTask"
			class="overview-drag-preview"
			:style="{left: `${Math.max(8, Math.min(dragPosition.x + 16, viewportWidth - 300))}px`, top: `${dragPosition.y + 20}px`}"
			role="status"
		>
			<strong>{{ draggedTask.title }}</strong>
			<span>{{ dragHint }}</span>
		</div>
	</Teleport>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'
import {useWindowSize} from '@vueuse/core'
import {useOverviewPointerDrag} from '@/composables/useOverviewPointerDrag'
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
const groups = computed(() => {
	const groups = groupOverviewTasks(props.tasks, props.ancestors)
	// Append empty/filtered destination projects without shifting existing rows.
	if (draggedTask.value) {
		const shown = new Set(groups.map(group => group.projectId))
		for (const project of Object.values(projectStore.projects)) {
			if (project.id > 0 && !shown.has(project.id) && canWrite(project.id)) {
				groups.push({projectId: project.id, rows: [], count: 0})
			}
		}
	}
	return groups
})
const collapsedProjects = ref(new Set<number>())
const collapsedTasks = ref(new Set<number>())
const projectTitle = (id: number) => projectStore.projects[id]?.title || `项目 #${id}`
const draggedTask = ref<ITask | null>(null)
const rootDropProject = ref<number | null>(null)
const saving = ref(false)
const taskDropTarget = ref<number | null>(null)
const dragHint = ref('')
let destination: {projectId: number, parentId: number} | null = null
let draggedSubtree = new Set<number>()
const {width: viewportWidth} = useWindowSize()
const canWrite = (id: number) => !projectStore.projects[id]?.isArchived
	&& (projectStore.projects[id]?.maxPermission ?? 0) > PERMISSIONS.READ

const {begin: beginPointerDrag, position: dragPosition} = useOverviewPointerDrag<ITask>({
	canStart: task => !saving.value && !props.loading && canWrite(task.projectId),
	start: task => {
		draggedTask.value = task
		draggedSubtree = taskSubtreeIds(task.id, [...props.tasks, ...props.ancestors])
	},
	move: updateDropTarget,
	drop: () => {
		if (destination) void moveTask(destination.projectId, destination.parentId)
		else if (dragHint.value) error({message: dragHint.value})
	},
	end: endDrag,
})
function endDrag() {
	draggedTask.value = null
	rootDropProject.value = null
	taskDropTarget.value = null
	destination = null
	draggedSubtree.clear()
}
function updateDropTarget(x: number, y: number) {
	rootDropProject.value = null
	taskDropTarget.value = null
	destination = null
	dragHint.value = '拖到任务或项目名下方的虚线框；按住鼠标可用滚轮滚动，Esc 取消。'
	const source = draggedTask.value
	const element = document.elementFromPoint(x, y)
	const projectElement = element?.closest('[data-overview-project]')
	if (!source || !projectElement) return
	const projectId = Number(projectElement.getAttribute('data-overview-project'))

	if (!canWrite(projectId)) { dragHint.value = '没有此项目的编辑权限。'; return }
	if (element?.closest('.overview-project-root-drop, .overview-project-heading')) {
		rootDropProject.value = projectId
		destination = {projectId, parentId: 0}
		dragHint.value = `松开放到“${projectTitle(projectId)}”下，作为顶层任务。`
		return
	}
	const row = element?.closest('[data-overview-task]')
	if (!row) return
	const parentId = Number(row.getAttribute('data-overview-task'))
	if (draggedSubtree.has(parentId)) {
		dragHint.value = '不能将任务移到自身或自己的子任务下。'
		return
	}
	taskDropTarget.value = parentId
	destination = {projectId, parentId}
	dragHint.value = '松开后成为此任务的子任务。'
}
async function moveTask(projectId: number, parentId: number) {
	const source = draggedTask.value
	rootDropProject.value = null
	if (!source || saving.value || props.loading) return

	if (parentId && taskSubtreeIds(source.id, [...props.tasks, ...props.ancestors]).has(parentId)) {
		error({message: '不能将任务移到自身或自己的子任务下。'})
		return
	}
	const parents = source.relatedTasks?.parenttask || []
	if (source.projectId === projectId && ((!parentId && !parents.length) || (parents.length === 1 && parents[0].id === parentId))) return
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
			body: {project_id: projectId, parent_id: parentId, before_task_id: 0, project_view_id: view.id},
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
:global(html.overview-task-dragging),
:global(html.overview-task-dragging *) {
	cursor: grabbing !important;
	user-select: none !important;
}
.overview-drag-preview {
	position: fixed;
	z-index: 10000;
	pointer-events: none;
	display: flex;
	flex-direction: column;
	gap: .25rem;
	max-inline-size: min(280px, 90vw);
	padding: .5rem .75rem;
	border: 1px solid var(--grey-300);
	border-radius: .25rem;
	background: var(--white);
	color: var(--text);
	font-size: .875rem;
	overflow-wrap: anywhere;
}
.overview-task-row.is-task-drop-target .overview-task-content {
	outline: 2px solid var(--primary);
	background: var(--grey-100);
}
.overview-task-row.is-drag-source {
	opacity: .65;
}
.overview-drag-help {
	margin: 0;
	padding: .75rem;
	font-size: .875rem;
	color: var(--text);
}
.overview-project-heading.is-drop-target,
.overview-project-root-drop.is-drop-target {
	outline: 2px solid var(--primary);
	outline-offset: 2px;
	background: var(--grey-100);
}
.overview-project-root-drop {
	display: flex;
	align-items: center;
	min-block-size: 2.5rem;
	margin-block: .5rem;
	padding: .5rem .75rem;
	border: 1px dashed var(--grey-300);
	border-radius: .25rem;
	color: var(--text);
	font-size: .875rem;
	overflow-wrap: anywhere;
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
