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
			按任务、子任务逐级显示，点击名称旁的按钮可展开或收起。进展按记录日期倒序显示；遗留事项取最新一条每日进展的填写内容。
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
			<header>
				<button
					type="button"
					class="hierarchy-toggle"
					:aria-expanded="isExpanded(group.root.id)"
					:aria-label="`${isExpanded(group.root.id) ? '收起' : '展开'}任务 ${group.root.title}`"
					@click="toggle(group.root.id)"
				>
					<svg
						viewBox="0 0 16 16"
						aria-hidden="true"
						:class="{expanded: isExpanded(group.root.id)}"
					><path d="m6 3 5 5-5 5" /></svg>
				</button><h3>{{ group.root.title }}</h3><span>{{ group.root.done ? '任务已完成' : '任务未完成' }} · {{ group.rows.length - 1 }} 个子任务</span>
			</header>
			<template v-if="isExpanded(group.root.id)">
				<ReadonlyRichText
					v-if="group.root.description"
					class="task-description"
					:html="group.root.description"
				/>
				<details class="task-own-progress">
					<summary>任务自身进展</summary>
					<div class="subtask-scroll">
						<table class="subtask-table">
							<thead><tr><th>任务名</th><th>任务描述</th><th>遗留事项</th><th>进展</th></tr></thead><tbody>
								<ProjectProgressRow
									:task="group.root"
									:descendants="group.rows.slice(1).map(row => row.task)"
									:depth="0"
								/>
							</tbody>
						</table>
					</div>
				</details>
				<div
					v-if="group.visibleRows.length"
					class="subtask-scroll"
					tabindex="0"
					:aria-label="`${group.root.title}子任务表格，可横向滚动`"
				>
					<table class="subtask-table">
						<colgroup><col style="width: 18%"><col style="width: 27%"><col style="width: 20%"><col style="width: 35%"></colgroup>
						<thead>
							<tr>
								<th scope="col">
									子任务名
								</th><th scope="col">
									子任务描述
								</th><th scope="col">
									遗留事项
								</th><th scope="col">
									进展
								</th>
							</tr>
						</thead>
						<tbody>
							<ProjectProgressRow
								v-for="row in group.visibleRows"
								:key="row.task.id"
								:task="row.task"
								:depth="row.depth"
								:has-children="parents.has(row.task.id)"
								:expanded="isExpanded(row.task.id)"
								@toggle="toggle(row.task.id)"
							/>
						</tbody>
					</table>
				</div>
				<p
					v-else
					class="task-description"
				>
					{{ group.rows.length === 1 ? '暂无子任务，可进入编辑模式添加。' : '没有符合筛选条件的子任务。' }}
				</p>
			</template>
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
import {useStorage} from '@vueuse/core'
import {visibleProgressRows, groupProgressTasks, type ProgressTask} from '@/helpers/projectProgress'
import ProjectProgressRow from './ProjectProgressRow.vue'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
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
const collapsed = useStorage<number[]>('tasktrace:overview-collapsed', [])
const collapsedIds = computed(() => new Set(collapsed.value))
const searchExpanded = ref(new Set<number>())
const parents = computed(() => new Set(grouped.value.flatMap(group => group.rows.filter((row, i, rows) => rows[i + 1]?.depth > row.depth).map(row => row.task.id))))
function isExpanded(id: number) {
	return search.value.trim() ? !searchExpanded.value.has(id) : !collapsedIds.value.has(id)
}
function toggle(id: number) {
	if (search.value.trim()) {
		const next = new Set(searchExpanded.value)
		if (next.has(id)) next.delete(id); else next.add(id)
		searchExpanded.value = next
		return
	}
	collapsed.value = collapsedIds.value.has(id) ? collapsed.value.filter(value => value !== id) : [...collapsed.value, id]
}
const groups = computed(() => grouped.value.map(group => {
	const matching = visibleProgressRows(group.rows, scope.value, search.value, new Set())
	const visible = visibleProgressRows(matching, 'all', '', search.value.trim() ? searchExpanded.value : collapsedIds.value)
	return {...group, matching, visibleRows: visible.filter(row => row.depth > 0)}
}).filter(group => group.matching.length))
watch(search, () => { searchExpanded.value = new Set() })
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
.hierarchy-toggle {
 border: 0;
 background: transparent;
 color: var(--grey-700);
 cursor: pointer;
 padding: .25rem;
 flex-shrink: 0;
 &:hover { background: var(--grey-200);
 }
 &:focus-visible { outline: 2px solid var(--primary);
 outline-offset: 2px;
 }
 svg { inline-size: 1rem;
 block-size: 1rem;
 fill: none;
 stroke: currentcolor;
 stroke-width: 2;
 display: block;
 }
 .expanded { transform: rotate(90deg);
 }
}
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
.progress-group h3 {
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
.subtask-scroll {
 overflow-x: auto;
}
.subtask-table {
 inline-size: 100%;
 min-inline-size: 760px;
 table-layout: fixed;
 border-collapse: collapse;
 thead th {
  padding: .6rem .75rem;
  text-align: start;
  font-size: .8125rem;
  border-block-start: 1px solid var(--grey-200);
  background: var(--grey-100);
 }
}
.task-description { padding: .5rem .8rem;
 }
.task-own-progress {
 padding: .5rem .8rem;
 font-size: .8125rem;
 summary { cursor: pointer;
 }
}
</style>
