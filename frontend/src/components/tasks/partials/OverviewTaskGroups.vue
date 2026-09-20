<template>
	<section
		v-for="project in groups"
		:key="project.projectId"
		class="overview-project"
		:data-overview-project="project.projectId"
		:aria-label="projectTitle(project.projectId)"
	>
		<header class="overview-project-heading">
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
						:can-mark-as-done="(projectStore.projects[row.task.projectId]?.maxPermission ?? 0) > PERMISSIONS.READ"
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
const props = defineProps<{tasks: ITask[], ancestors: ITask[]}>()
const emit = defineEmits<{taskUpdated: [task: ITask]}>()
const projectStore = useProjectStore()
const groups = computed(() => groupOverviewTasks(props.tasks, props.ancestors))
const collapsedProjects = ref(new Set<number>())
const collapsedTasks = ref(new Set<number>())
const projectTitle = (id: number) => projectStore.projects[id]?.title || `项目 #${id}`
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
