<template>
	<section
		class="project-progress"
		aria-label="项目展示模式"
	>
		<h2 class="project-overview-title">
			项目总表
		</h2>
		<div class="progress-toolbar">
			<p>{{ grouped.length }} 个任务 · {{ tasks.length - grouped.length }} 个子任务 · 合计已完成 {{ completed }} / {{ tasks.length }} 项</p>
			<label>查找事项 <input
				v-model="search"
				class="input"
				type="search"
				placeholder="按名称查找分区"
				aria-label="查找项目事项"
			></label>
			<label>显示 <select
				v-model="scope"
				class="input"
			><option value="all">全部事项</option><option value="pending">未完成</option><option value="done">已完成</option></select></label>
			<XButton
				variant="secondary"
				:disabled="loading"
				@click="load"
			>
				刷新进展
			</XButton>
		</div>
		<p class="browse-hint">
			项目总表包含多个任务，每个任务下面可细分子任务。点击任务或子任务可展开说明、进展和图片。
		</p>
		<p
			v-if="loading"
			role="status"
		>
			正在读取项目事项…
		</p>
		<p
			v-if="error"
			role="alert"
		>
			{{ error }} <XButton
				variant="secondary"
				@click="load"
			>
				重试
			</XButton>
		</p>
		<p v-if="!loading && !error && !groups.length">
			{{ tasks.length ? '没有匹配的事项，请调整筛选。' : '项目还没有事项，进入编辑模式后即可添加。' }}
		</p>
		<section
			v-for="group in visibleGroups"
			:key="`${revision}-${group.root.id}`"
			class="progress-group"
		>
			<header><span>任务及子任务</span><span>{{ group.rows.filter(row => row.task.done).length }} / {{ group.rows.length }} 项完成</span></header>
			<ProjectProgressRow
				v-for="row in group.visibleRows"
				:key="row.task.id"
				:task="row.task"
				:depth="row.depth"
			/>
		</section>
		<nav
			v-if="groups.length > 20"
			aria-label="项目分区翻页"
			class="progress-pages"
		>
			<XButton
				variant="secondary"
				:disabled="page === 1"
				@click="page--"
			>
				上一页
			</XButton>
			<span>第 {{ page }} / {{ Math.ceil(groups.length / 20) }} 页</span>
			<XButton
				variant="secondary"
				:disabled="page * 20 >= groups.length"
				@click="page++"
			>
				下一页
			</XButton>
		</nav>
	</section>
</template>

<script setup lang="ts">
import {ref, computed, watch, onBeforeUnmount} from 'vue'
import {projectTasksList} from '@/client/generated'
import {groupProgressTasks, type ProgressTask} from '@/helpers/projectProgress'
import ProjectProgressRow from './ProjectProgressRow.vue'
const props = defineProps<{projectId: number}>()
const tasks = ref<ProgressTask[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')
const scope = ref('all')
const page = ref(1)
const revision = ref(0)
let requestId = 0
const completed = computed(() => tasks.value.filter(task => task.done).length)
const grouped = computed(() => groupProgressTasks(tasks.value))
const groups = computed(() => grouped.value.filter(group => !search.value.trim() || group.rows.some(row => row.task.title?.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase()))).map(group => ({...group, visibleRows: group.rows.filter(row => scope.value === 'all' || (scope.value === 'done' ? row.task.done : !row.task.done))})).filter(group => group.visibleRows.length))
const visibleGroups = computed(() => groups.value.slice((page.value - 1) * 20, page.value * 20))
watch([search, scope], () => { page.value = 1 })
async function load() {
	const version = ++requestId
	loading.value = true; error.value = ''; tasks.value = []; page.value = 1
	try {
		const collected: ProgressTask[] = []
		for (let next = 1; ; next++) {
			const result = await projectTasksList({path: {project: props.projectId}, query: {page: next, per_page: 100, sort_by: ['id'], order_by: ['asc'], expand: ['comment_count']}})
			if (version !== requestId) return
			const items = (result.data.items || []).filter((task): task is ProgressTask => typeof task.id === 'number')
			collected.push(...items)
			if (next >= (result.data.total_pages || 1) || items.length === 0) break
		}
		tasks.value = [...new Map(collected.map(task => [task.id, task])).values()]; revision.value++
	} catch { if (version === requestId) error.value = '项目读取失败，请重试。' }
	finally { if (version === requestId) loading.value = false }
}
watch(() => props.projectId, load, {immediate: true})
onBeforeUnmount(() => requestId++)
</script>

<style scoped lang="scss">
.project-overview-title {
	font-size: 1.125rem;
	margin-block-end: .75rem;
}

.progress-toolbar {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: .75rem;
	margin-block-end: .5rem;
	}
.progress-toolbar p {
	margin: 0;
	margin-inline-end: auto;
	font-size: .875rem;
	}
.progress-toolbar label {
	display: flex;
	align-items: center;
	gap: .5rem;
	font-size: .875rem;
	}
.progress-toolbar .input {
	inline-size: auto;
	max-inline-size: 15rem;
	}
.browse-hint {
	color: var(--grey-600);
	font-size: .8125rem;
	margin-block-end: 1rem;
	}
.progress-group {
	border: 1px solid var(--grey-200);
	border-radius: .375rem;
	background: var(--white);
	margin-block-end: 1rem;
	}
.progress-group header {
	display: flex;
	align-items: center;
	gap: 1rem;
	padding: .7rem .8rem;
	background: var(--grey-100);
	}
.progress-group header > span:first-child {
	font-size: 1rem;
	margin: 0;
	flex: 1;
	}
.progress-group header span {
	font-size: .75rem;
	color: var(--grey-600);
	}
.progress-pages {
	display: flex;
	justify-content: center;
	gap: 1rem;
	align-items: center;
	margin-block: 1rem;
	}
@media (width <= 700px) {
.progress-toolbar label {
	flex-wrap: wrap;
	max-inline-size: 100%;
	}
.progress-toolbar .input {
	min-inline-size: 0;
	max-inline-size: 100%;
	}
.progress-group header {
	flex-wrap: wrap;
	}
}
</style>
