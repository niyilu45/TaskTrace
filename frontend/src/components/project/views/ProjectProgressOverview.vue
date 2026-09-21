<template>
	<section
		ref="overview"
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
			<div
				ref="peopleFilter"
				class="people-filter"
			>
				<span class="people-filter__label">人员</span>
				<button
					type="button"
					class="input people-filter__trigger"
					:aria-expanded="peopleMenuOpen"
					aria-haspopup="true"
					:aria-label="`按所属人或受理人筛选，${peopleFilterLabel}`"
					@click="peopleMenuOpen = !peopleMenuOpen"
				>
					<span>{{ peopleFilterLabel }}</span>
					<Icon
						icon="chevron-down"
						:class="{'is-expanded': peopleMenuOpen}"
					/>
				</button>
				<div
					v-if="peopleMenuOpen"
					class="people-filter__menu"
					role="group"
					aria-label="选择人员，可多选"
				>
					<div class="people-filter__actions">
						<button
							type="button"
							:disabled="!currentPersonKey"
							@click="selectOnlyCurrentPerson"
						>
							只看自己
						</button>
						<button
							type="button"
							:disabled="!peopleOptions.length"
							@click="selectAllPeople"
						>
							全部人员
						</button>
					</div>
					<label
						v-for="person in peopleOptions"
						:key="person.key"
						class="people-filter__option"
						:title="teamStore.identityTitleFor(person.username)"
					>
						<input
							type="checkbox"
							:checked="selectedPersonKeys.includes(person.key)"
							@change="togglePerson(person.key)"
						>
						<img
							v-if="person.avatar"
							:src="person.avatar"
							alt=""
						>
						<span
							v-else
							class="people-filter__avatar"
						>{{ person.initials }}</span>
						<span class="people-filter__name">
							<strong>{{ person.name }}</strong>
						</span>
						<small v-if="person.key === currentPersonKey">自己</small>
					</label>
					<p v-if="!peopleOptions.length">
						暂无可筛选人员
					</p>
				</div>
			</div>
			<label>最近进展 <select
				v-model="activityRange"
				class="input"
				aria-label="按最近有进展的天数筛选事项"
				@change="changeActivityRange"
			><option value="all">不限</option><option value="1">1 天内</option><option value="7">7 天内</option><option value="30">30 天内</option><option value="custom">自定义</option></select></label>
			<label v-if="activityRange === 'custom'">最近 <input
				v-model="activityDaysInput"
				class="input progress-days-input"
				type="number"
				min="1"
				max="36500"
				step="1"
				aria-label="筛选多少天内有进展的事项"
				:aria-invalid="!!activityFilterError"
				@change="applyActivityDays"
				@keydown.enter.prevent="applyActivityDays"
			> 天内有进展</label>
			<label>进展范围 <select
				v-model="progressRange"
				class="input"
				aria-label="进展显示范围"
				@change="changeProgressRange"
			><option value="all">全部</option><option value="1">最近 1 天</option><option value="7">最近 7 天</option><option value="30">最近 30 天</option><option value="custom">自定义</option></select></label>
			<label v-if="progressRange === 'custom'">显示 <input
				v-model="progressDaysInput"
				class="input progress-days-input"
				type="number"
				min="1"
				max="36500"
				step="1"
				aria-label="进展显示天数"
				:aria-invalid="!!progressRangeError"
				:aria-describedby="`progress-range-hint-${projectId}`"
				@change="applyProgressDays"
				@keydown.enter.prevent="applyProgressDays"
			> 天</label>
			<XButton
				variant="secondary"
				:disabled="loading"
				@click="load({preserveView: true})"
			>
				刷新进展
			</XButton>
			<XButton
				variant="secondary"
				:disabled="!customWidths"
				@click="resetColumns"
			>
				恢复默认列宽
			</XButton>
		</div>
		<p
			v-if="columnsStorageError"
			role="status"
		>
			{{ columnsStorageError }}
		</p>
		<p
			v-if="progressRangeError"
			role="alert"
		>
			{{ progressRangeError }}
		</p>
		<p
			v-if="progressRangeStorageError"
			role="status"
		>
			{{ progressRangeStorageError }}
		</p>
		<p
			v-if="progressActivityLoading"
			role="status"
		>
			正在检查各事项最近的每日进展…
		</p>
		<p
			v-if="activityFilterError"
			role="alert"
		>
			{{ activityFilterError }} <XButton
				v-if="recentProgressDays > 0"
				variant="secondary"
				@click="loadProgressActivity()"
			>
				重试筛选
			</XButton>
		</p>
		<p
			v-if="recentProgressDays > 0"
			class="browse-hint"
		>
			仅显示最近 {{ recentProgressDays }} 个自然日内填写过每日进展的事项，并保留其父任务作为层级上下文；未命中的同级任务不会显示。
		</p>
		<p
			v-if="personFilterActive"
			class="browse-hint"
		>
			仅显示对所选协作人员可见的事项，并保留其父任务作为层级上下文；未命中的同级任务不会显示。
		</p>
		<p
			:id="`progress-range-hint-${projectId}`"
			class="browse-hint"
		>
			{{ progressDays ? `各事项以自己的最新进展日期为起点，显示含当天的最近 ${progressDays} 个自然日，空白日期也计入。` : '进展范围：显示全部历史记录。' }}
		</p>
		<p class="browse-hint">
			按任务、子任务逐级显示，点击名称旁的按钮可展开或收起。进展按记录日期倒序显示；遗留事项取最新一条每日进展的填写内容。拖动表头右侧分隔线可调整列宽，自动记住本项目的设置。
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
				@click="load({preserveView: true})"
			>
				重试
			</XButton>
		</p>
		<p v-if="!loading && !progressActivityLoading && !error && !groups.length">
			{{ tasks.length ? '没有匹配的事项，请调整筛选。' : '项目还没有事项，进入编辑模式后即可添加。' }}
		</p>
		<section
			v-for="group in visibleGroups"
			:key="`${revision}-${group.root.id}`"
			class="progress-group"
		>
			<header :data-progress-anchor="`group-${group.root.id}`">
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
				</button><div class="progress-group__title">
					<h3>
						<button
							type="button"
							class="task-edit-link"
							@click="openTaskEditor(group.root.id)"
						>
							{{ group.root.title }}
						</button>
					</h3>
					<TaskCollaborationMembers :task-id="group.root.id" />
				</div><span>任务{{ taskStatusLabel(group.root.status, group.root.done) }} · {{ group.rows.length - 1 }} 个子任务</span>
			</header>
			<template v-if="isExpanded(group.root.id)">
				<ReadonlyRichText
					v-if="group.root.description"
					class="task-description"
					:html="group.root.description"
				/>
				<details class="task-own-progress">
					<summary>任务自身进展</summary>
					<ProjectProgressTable
						:labels="['任务名', '任务描述', '遗留事项', '进展']"
						:widths="columnWidths"
						:label="`${group.root.title}任务自身进展`"
						@resize="customWidths = $event"
						@resized="saveColumns"
					>
						<ProjectProgressRow
							:task="group.root"
							:anchor="false"
							:descendants="group.matching.slice(1).map(row => row.task)"
							:depth="0"
							:progress-days="progressDays"
							:show-collaboration="false"
							@edit="openTaskEditor"
						/>
					</ProjectProgressTable>
				</details>
				<ProjectProgressTable
					v-if="group.visibleRows.length"
					:labels="['子任务名', '子任务描述', '遗留事项', '进展']"
					:widths="columnWidths"
					:label="`${group.root.title}子任务表格`"
					@resize="customWidths = $event"
					@resized="saveColumns"
				>
					<ProjectProgressRow
						v-for="row in group.visibleRows"
						:key="row.task.id"
						:task="row.task"
						:depth="row.depth"
						:progress-days="progressDays"
						:has-children="parents.has(row.task.id)"
						:expanded="isExpanded(row.task.id)"
						@toggle="toggle(row.task.id)"
						@edit="openTaskEditor"
					/>
				</ProjectProgressTable>
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
import {ref, computed, watch, onBeforeUnmount, nextTick} from 'vue'
import {useRoute, useRouter} from 'vue-router'
import {projectTasksList, taskCommentsList, type TaskComment} from '@/client/generated'
import {onClickOutside, useElementSize, useStorage} from '@vueuse/core'
import {visibleProgressRows, groupProgressTasks, latestProgressDate, progressTaskPeople, queueProgressRead, recentProgressTaskIds, type ProgressTask} from '@/helpers/projectProgress'
import {sortProgressNotes} from '@/helpers/progressNotes'
import ProjectProgressRow from './ProjectProgressRow.vue'
import ProjectProgressTable from './ProjectProgressTable.vue'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
import {taskStatusLabel} from '@/types/ITaskStatus'
import TaskCollaborationMembers from '@/components/tasks/partials/TaskCollaborationMembers.vue'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import {useAuthStore} from '@/stores/auth'
import Icon from '@/components/misc/Icon'
import {captureTasktraceScrollAnchor, tasktraceScrollAnchorDelta, type TasktraceScrollAnchor} from '@/helpers/tasktraceScrollAnchor'
import {readProjectProgressCache, writeProjectProgressCache} from '@/helpers/projectProgressCache'
const props = defineProps<{projectId: number}>()
const route = useRoute()
const router = useRouter()
const teamStore = useTasktraceTeamStore()
const authStore = useAuthStore()
const overview = ref<HTMLElement>()
const {width: overviewWidth} = useElementSize(overview)
const customWidths = ref<number[] | null>(null)
const columnsStorageError = ref('')
const columnsStorageKey = computed(() => `tasktrace:progress-columns:${props.projectId}`)
const columnWidths = computed(() => customWidths.value || [0.18, 0.27, 0.20, 0.35].map(ratio => Math.round(Math.max(800, overviewWidth.value - 2) * ratio)))
watch(columnsStorageKey, key => {
	customWidths.value = null
	columnsStorageError.value = ''
	try {
		const saved: unknown = JSON.parse(localStorage.getItem(key) || 'null')
		if (Array.isArray(saved) && saved.length === 4 && saved.every((width, index) => typeof width === 'number' && Number.isFinite(width) && width >= (index === 0 ? 144 : 96) && width <= 1600)) {
			customWidths.value = saved.map(Math.round)
		}
	} catch { /* Invalid or unavailable storage falls back to responsive defaults. */ }
}, {immediate: true})
function saveColumns() {
	try {
		if (customWidths.value) localStorage.setItem(columnsStorageKey.value, JSON.stringify(customWidths.value))
		else localStorage.removeItem(columnsStorageKey.value)
		columnsStorageError.value = ''
	} catch { columnsStorageError.value = '列宽已调整，但当前浏览器无法保存设置。' }
}
function resetColumns() {
	customWidths.value = null
	saveColumns()
}
const progressRange = ref('all')
const customProgressDays = ref(7)
const progressDaysInput = ref('7')
const progressRangeError = ref('')
const progressRangeStorageError = ref('')
const progressRangeKey = computed(() => `tasktrace:progress-range:${props.projectId}`)
const progressDays = computed(() => progressRange.value === 'all' ? 0 : progressRange.value === 'custom' ? customProgressDays.value : Number(progressRange.value))
function validProgressDays(days: unknown): days is number {
	return typeof days === 'number' && Number.isSafeInteger(days) && days >= 1 && days <= 36500
}
watch(progressRangeKey, key => {
	progressRange.value = 'all'
	customProgressDays.value = 7
	progressDaysInput.value = '7'
	progressRangeError.value = ''
	progressRangeStorageError.value = ''
	try {
		const saved = JSON.parse(localStorage.getItem(key) || 'null')
		if (saved && ['all', '1', '7', '30', 'custom'].includes(saved.mode) && validProgressDays(saved.days)) {
			progressRange.value = saved.mode
			customProgressDays.value = saved.days
			progressDaysInput.value = String(saved.days)
		}
	} catch { /* Keep all progress visible when saved settings cannot be read. */ }
}, {immediate: true})
function saveProgressRange() {
	try {
		localStorage.setItem(progressRangeKey.value, JSON.stringify({mode: progressRange.value, days: customProgressDays.value}))
		progressRangeStorageError.value = ''
	} catch { progressRangeStorageError.value = '显示范围已调整，但当前浏览器无法保存设置。' }
}
function changeProgressRange() {
	progressRangeError.value = ''
	progressDaysInput.value = String(customProgressDays.value)
	saveProgressRange()
}
function applyProgressDays() {
	const days = Number(progressDaysInput.value)
	if (!validProgressDays(days)) {
		progressRangeError.value = `请输入 1～36500 之间的整数天数。当前仍显示最近 ${customProgressDays.value} 天。`
		return
	}
	customProgressDays.value = days
	progressDaysInput.value = String(days)
	progressRangeError.value = ''
	saveProgressRange()
}
const tasks = ref<ProgressTask[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')
const scope = ref('all')
const activityRange = ref('all')
const customActivityDays = ref(7)
const activityDaysInput = ref('7')
const activityFilterError = ref('')
const progressActivityLoading = ref(false)
const latestProgressDates = ref<Record<number, string>>({})
const progressActivityReady = ref(false)
const page = ref(1)
const revision = ref(0)
let requestId = 0
let progressActivityRequestId = 0
const recentProgressDays = computed(() => activityRange.value === 'all' ? 0 : activityRange.value === 'custom' ? customActivityDays.value : Number(activityRange.value))
const recentProgressMatches = computed(() => recentProgressDays.value === 0 ? null : progressActivityReady.value ? recentProgressTaskIds(latestProgressDates.value, recentProgressDays.value) : new Set<number>())
const peopleFilter = ref<HTMLElement>()
const peopleMenuOpen = ref(false)
const selectedPersonKeys = ref<string[]>([])
const personSelectionDirty = ref(false)
const currentUsername = computed(() => teamStore.status.username?.trim() || authStore.info?.username?.trim() || '')
const currentPersonKey = computed(() => teamStore.memberKey(currentUsername.value))
const peopleOptions = computed(() => {
	const people = new Map<string, {key: string, username: string, name: string, avatar: string, initials: string}>()
	const names = new Map<string, string>()
	for (const task of tasks.value) {
		for (const user of [task.created_by, ...(task.assignees ?? [])]) {
			const username = user?.username?.trim()
			if (username) names.set(teamStore.memberKey(username), user?.name?.trim() || username)
		}
	}
	const add = (username: string) => {
		const value = username.trim()
		if (!value) return
		const key = teamStore.memberKey(value)
		if (people.has(key)) return
		const avatar = teamStore.avatarFor(value)
		const name = names.get(key) || teamStore.displayNameFor(value)
		people.set(key, {key, username: value, name, avatar, initials: name.slice(0, 2).toLocaleUpperCase() || '?'})
	}
	add(currentUsername.value)
	for (const username of teamStore.memberRoster) add(username)
	for (const task of tasks.value) {
		for (const username of progressTaskPeople(task, teamStore.bindingForTask(task.id), currentUsername.value)) add(username)
	}
	return [...people.values()].sort((left, right) => {
		if (left.key === currentPersonKey.value) return -1
		if (right.key === currentPersonKey.value) return 1
		return left.name.localeCompare(right.name, 'zh-CN')
	})
})
const allPersonKeys = computed(() => peopleOptions.value.map(person => person.key))
const personFilterActive = computed(() => {
	if (!allPersonKeys.value.length) return false
	const selected = new Set(selectedPersonKeys.value)
	return selected.size !== allPersonKeys.value.length || allPersonKeys.value.some(key => !selected.has(key))
})
const peopleFilterLabel = computed(() => {
	const selected = peopleOptions.value.filter(person => selectedPersonKeys.value.includes(person.key))
	if (!selected.length) return '未选择人员'
	if (selected.length === peopleOptions.value.length && selected.length > 1) return `全部 ${selected.length} 人`
	if (selected.length === 1) return selected[0].key === currentPersonKey.value ? '我自己' : selected[0].name
	return `已选 ${selected.length} 人`
})
const personMatches = computed<ReadonlySet<number> | null>(() => {
	if (!personFilterActive.value) return null
	const selected = new Set(selectedPersonKeys.value)
	return new Set(tasks.value.filter(task => progressTaskPeople(task, teamStore.bindingForTask(task.id), currentUsername.value)
		.some(username => selected.has(teamStore.memberKey(username)))).map(task => task.id))
})
const eligibleTaskIds = computed<ReadonlySet<number> | null>(() => {
	const filters = [recentProgressMatches.value, personMatches.value].filter((value): value is ReadonlySet<number> => value !== null)
	if (!filters.length) return null
	return new Set(tasks.value.map(task => task.id).filter(id => filters.every(filter => filter.has(id))))
})
function togglePerson(key: string) {
	personSelectionDirty.value = true
	selectedPersonKeys.value = selectedPersonKeys.value.includes(key)
		? selectedPersonKeys.value.filter(value => value !== key)
		: [...selectedPersonKeys.value, key]
}
function selectOnlyCurrentPerson() {
	personSelectionDirty.value = true
	selectedPersonKeys.value = currentPersonKey.value ? [currentPersonKey.value] : []
}
function selectAllPeople() {
	personSelectionDirty.value = true
	selectedPersonKeys.value = allPersonKeys.value
}
onClickOutside(peopleFilter, () => { peopleMenuOpen.value = false })
const completed = computed(() => tasks.value.filter(task => task.done).length)
const grouped = computed(() => groupProgressTasks(tasks.value))
const collapsed = useStorage<number[]>('tasktrace:overview-collapsed', [])
const collapsedIds = computed(() => new Set(collapsed.value))
const searchExpanded = ref(new Set<number>())
const hierarchyFilterActive = computed(() => !!search.value.trim() || scope.value !== 'all' || recentProgressDays.value > 0 || personFilterActive.value)
function isExpanded(id: number) {
	return hierarchyFilterActive.value ? !searchExpanded.value.has(id) : !collapsedIds.value.has(id)
}
function toggle(id: number) {
	if (hierarchyFilterActive.value) {
		const next = new Set(searchExpanded.value)
		if (next.has(id)) next.delete(id); else next.add(id)
		searchExpanded.value = next
		return
	}
	collapsed.value = collapsedIds.value.has(id) ? collapsed.value.filter(value => value !== id) : [...collapsed.value, id]
}
const groups = computed(() => grouped.value.map(group => {
	const matching = visibleProgressRows(group.rows, scope.value, search.value, new Set(), eligibleTaskIds.value)
	const visible = visibleProgressRows(matching, 'all', '', hierarchyFilterActive.value ? searchExpanded.value : collapsedIds.value)
	return {...group, matching, visibleRows: visible.filter(row => row.depth > 0)}
}).filter(group => group.matching.length))
const parents = computed(() => new Set(groups.value.flatMap(group => group.matching.filter((row, i, rows) => rows[i + 1]?.depth > row.depth).map(row => row.task.id))))
watch([search, scope, recentProgressDays, selectedPersonKeys], () => { searchExpanded.value = new Set() })
const visibleGroups = computed(() => groups.value.slice((page.value - 1) * 20, page.value * 20))
watch([search, scope, recentProgressDays, selectedPersonKeys], () => { page.value = 1 })

async function loadProgressActivity(sourceTasks = tasks.value) {
	if (recentProgressDays.value <= 0) return
	const version = ++progressActivityRequestId
	const today = new Date()
	progressActivityLoading.value = true
	activityFilterError.value = ''
	try {
		const entries = await Promise.all(sourceTasks.map(async task => {
			if (task.comment_count === 0) return [task.id, ''] as const
			const history: TaskComment[] = []
			for (let next = 1; ; next++) {
				const result = await queueProgressRead(() => taskCommentsList({path: {task: task.id}, query: {page: next, per_page: 100, order_by: 'desc'}}))
				if (version !== progressActivityRequestId) return [task.id, ''] as const
				const items = result.data.items || []
				history.push(...items)
				if (next >= (result.data.total_pages || 1) || items.length === 0) break
			}
			return [task.id, latestProgressDate(sortProgressNotes(history), today)] as const
		}))
		if (version !== progressActivityRequestId) return
		latestProgressDates.value = Object.fromEntries(entries)
		progressActivityReady.value = true
	} catch {
		if (version === progressActivityRequestId) {
			progressActivityReady.value = false
			activityFilterError.value = '最近进展筛选读取失败，请重试。'
		}
	} finally {
		if (version === progressActivityRequestId) progressActivityLoading.value = false
	}
}
function changeActivityRange() {
	activityFilterError.value = ''
	activityDaysInput.value = String(customActivityDays.value)
	if (recentProgressDays.value === 0) {
		progressActivityRequestId++
		progressActivityLoading.value = false
		return
	}
	if (!progressActivityReady.value) void loadProgressActivity()
}
function applyActivityDays() {
	const days = Number(activityDaysInput.value)
	if (!validProgressDays(days)) {
		activityFilterError.value = `请输入 1～36500 之间的整数天数。当前仍筛选最近 ${customActivityDays.value} 天。`
		return
	}
	customActivityDays.value = days
	activityDaysInput.value = String(days)
	activityFilterError.value = ''
	if (!progressActivityReady.value) void loadProgressActivity()
}
function captureViewAnchor() {
	return captureTasktraceScrollAnchor(overview.value, window.scrollY, window.innerHeight)
}
async function restoreViewAnchor(anchor: TasktraceScrollAnchor | null) {
	if (!anchor) return
	await nextTick()
	await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
	const delta = tasktraceScrollAnchorDelta(overview.value, anchor)
	if (delta === null) window.scrollTo(0, anchor.scrollY)
	else if (delta) window.scrollBy(0, delta)
}
async function load(options: {preserveView?: boolean, anchor?: TasktraceScrollAnchor | null} = {}) {
	const viewAnchor = options.preserveView ? options.anchor ?? captureViewAnchor() : null
	const version = ++requestId
	const projectId = props.projectId
	progressActivityRequestId++
	progressActivityLoading.value = false
	loading.value = true; error.value = ''
	if (!options.preserveView) {
		tasks.value = readProjectProgressCache(projectId)
		page.value = 1
	}
	if (isLocalBuild) void teamStore.refresh().catch(() => { /* The task list remains usable while a LAN repository is offline. */ })
	try {
		const collected: ProgressTask[] = []
		for (let next = 1; ; next++) {
			const result = await projectTasksList({path: {project: projectId}, query: {page: next, per_page: 100, sort_by: ['id'], order_by: ['asc'], expand: ['comment_count']}})
			if (version !== requestId) return
			const items = (result.data.items || []).filter((task): task is ProgressTask => typeof task.id === 'number')
			collected.push(...items)
			tasks.value = [...new Map(collected.map(task => [task.id, task])).values()]
			revision.value++
			if (next >= (result.data.total_pages || 1) || items.length === 0) break
		}
		writeProjectProgressCache(projectId, tasks.value)
		progressActivityRequestId++
		progressActivityReady.value = false
		latestProgressDates.value = {}
		if (recentProgressDays.value > 0) await loadProgressActivity(tasks.value)
	} catch { if (version === requestId) error.value = '项目读取失败，请重试。' }
	finally {
		if (version === requestId) {
			loading.value = false
			await restoreViewAnchor(viewAnchor)
		}
	}
}
let modalOpenedHere = false
let savedViewAnchor: TasktraceScrollAnchor | null = null
function openTaskEditor(taskId: number) {
	savedViewAnchor = captureViewAnchor()
	modalOpenedHere = true
	void router.push({
		name: 'task.detail',
		params: {id: taskId},
		state: {backdropView: route.fullPath},
	})
}
watch(() => route.name, async name => {
	if (!modalOpenedHere) return
	if (name === 'task.detail') {
		await restoreViewAnchor(savedViewAnchor)
		return
	}
	modalOpenedHere = false
	await load({preserveView: true, anchor: savedViewAnchor})
	savedViewAnchor = null
})
watch(() => props.projectId, () => {
	personSelectionDirty.value = false
	selectedPersonKeys.value = currentPersonKey.value ? [currentPersonKey.value] : []
	peopleMenuOpen.value = false
	void load()
}, {immediate: true})
watch(currentPersonKey, (value, previous) => {
	if (!personSelectionDirty.value && value !== previous) selectedPersonKeys.value = value ? [value] : []
})
onBeforeUnmount(() => { requestId++; progressActivityRequestId++ })
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
.progress-toolbar .progress-days-input {
 inline-size: 6rem;
}
.people-filter {
	position: relative;
	display: flex;
	align-items: center;
	gap: .5rem;
	font-size: .875rem;
}
.people-filter__trigger {
	display: inline-flex;
	align-items: center;
	justify-content: space-between;
	gap: .65rem;
	min-inline-size: 8rem;
	min-block-size: 2.25rem;
	padding: .35rem .65rem;
	border: 1px solid var(--grey-300);
	border-radius: .25rem;
	background: var(--white);
	color: var(--text);
	cursor: pointer;
	.icon {
		font-size: .7rem;
		transition: transform $transition;
		&.is-expanded { transform: rotate(180deg); }
	}
	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: 2px;
	}
}
.people-filter__menu {
	position: absolute;
	z-index: 30;
	inset-block-start: calc(100% + .35rem);
	inset-inline-end: 0;
	inline-size: min(20rem, calc(100vw - 2rem));
	max-block-size: 22rem;
	overflow-y: auto;
	padding: .45rem;
	border: 1px solid var(--grey-200);
	border-radius: .5rem;
	background: var(--white);
	box-shadow: 0 12px 30px rgb(17 24 39 / 18%);
	> p {
		margin: .5rem;
		color: var(--grey-600);
	}
}
.people-filter__actions {
	display: flex;
	justify-content: space-between;
	gap: .5rem;
	padding: .15rem .25rem .45rem;
	border-block-end: 1px solid var(--grey-200);
	button {
		padding: .25rem;
		border: 0;
		background: transparent;
		color: var(--primary);
		cursor: pointer;
		font: inherit;
		font-size: .8rem;
		&:disabled {
			color: var(--grey-400);
			cursor: default;
		}
	}
}
.people-filter__option {
	display: flex;
	align-items: center;
	gap: .55rem;
	min-block-size: 2.6rem;
	padding: .35rem .4rem;
	border-radius: .35rem;
	cursor: pointer;
	&:hover { background: var(--grey-100); }
	input { flex: 0 0 auto; }
	img,
	.people-filter__avatar {
		inline-size: 1.65rem;
		block-size: 1.65rem;
		border-radius: 50%;
		flex: 0 0 auto;
	}
	img { object-fit: cover; }
	.people-filter__avatar {
		display: inline-grid;
		place-items: center;
		background: var(--primary);
		color: var(--white);
		font-size: .65rem;
		font-weight: 700;
	}
	> small {
		margin-inline-start: auto;
		color: var(--grey-500);
	}
}
.people-filter__name {
	display: flex;
	min-inline-size: 0;
	flex: 1;
	flex-direction: column;
	strong,
	small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	small { color: var(--grey-500); }
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
.progress-group__title {
	flex: 1;
	min-inline-size: 0;
}
.progress-group h3 {
	font-size: 1rem;
	margin: 0;
	}
.task-edit-link {
	border: 0;
	background: transparent;
	color: var(--text);
	font: inherit;
	font-weight: inherit;
	padding: 0;
	cursor: pointer;
	text-align: start;
	text-decoration: underline;
	text-decoration-color: transparent;
	text-underline-offset: .18em;
	&:hover {
		color: var(--primary);
		text-decoration-color: currentcolor;
	}
	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: 2px;
	}
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
.project-progress {
 min-inline-size: 0;
 max-inline-size: 100%;
}
.task-description {
	min-inline-size: 0;
	max-inline-size: 100%;
	overflow: hidden;
	padding: .5rem .8rem;
}
.task-own-progress {
 padding-block: .5rem;
 font-size: .8125rem;
 summary { cursor: pointer;
 padding-inline: .8rem;
 padding-block-end: .5rem;
 }
}
</style>
