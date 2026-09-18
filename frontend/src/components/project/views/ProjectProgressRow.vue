<template>
	<tr
		ref="element"
		class="progress-row"
		:data-task-id="task.id"
	>
		<th
			scope="row"
			:style="{'padding-inline-start': `${Math.max(depth - 1, 0) * 1.25 + .75}rem`}"
		>
			<div class="task-name">
				<button
					v-if="hasChildren"
					type="button"
					class="row-toggle"
					:aria-expanded="expanded"
					:aria-label="`${expanded ? '收起' : '展开'}子任务 ${task.title}`"
					@click="$emit('toggle')"
				>
					<svg
						viewBox="0 0 16 16"
						aria-hidden="true"
						:class="{expanded}"
					><path d="m6 3 5 5-5 5" /></svg>
				</button><span
					v-else
					class="toggle-spacer"
					aria-hidden="true"
				/>
				<span>{{ task.title }}</span>
			</div>
			<small>{{ taskStatusLabel(task.status, task.done) }}<template v-if="depth > 1"> · 下级子任务</template></small>
		</th>
		<td>
			<ReadonlyRichText
				v-if="task.description"
				:html="task.description"
			/><span
				v-else
				class="empty"
			>暂无描述</span>
		</td>
		<td class="outstanding-cell">
			<strong v-if="descendants?.length">任务自身</strong>
			<span
				v-if="loading"
				class="empty"
			>读取中…</span>
			<span
				v-else-if="error"
				class="empty"
			>遗留事项尚未加载</span>
			<ReadonlyRichText
				v-else-if="outstanding"
				:html="outstanding"
			/>
			<span
				v-else
				class="empty"
			>暂无遗留事项</span>
			<SubtaskOutstandingSummary
				v-if="descendants?.length"
				:tasks="descendants"
			/>
		</td>
		<td class="progress-cell">
			<p
				v-if="loading"
				role="status"
			>
				正在读取进展…
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
			<span
				v-if="!loading && !error && !notes.length"
				class="empty"
			>暂无进展</span>
			<div
				v-for="note in notes"
				:key="note.id"
				class="history-entry"
			>
				<time>{{ note.date }}<template v-if="showAuthors(note.date)"> · {{ authorsByDate[note.date].join('、') }}</template>：</time>
				<strong v-if="showAuthors(note.date)" class="progress-author">{{ note.author }}：</strong><ReadonlyRichText :html="note.progress" />
				<ProgressBacklinks :items="progressBacklinkMap[note.id || 0] || []" />
			</div>
			<button
				v-if="hiddenNotesCount > 0"
				type="button"
				class="range-summary"
				:aria-expanded="showAllProgress"
				@click="showAllProgress = !showAllProgress"
			>
				{{ showAllProgress ? '收起较早进展' : `已隐藏 ${hiddenNotesCount} 条较早进展，点击展开` }}
			</button>
		</td>
	</tr>
</template>

<script setup lang="ts">
import {ref, computed, onBeforeUnmount, watch} from 'vue'
import {useIntersectionObserver} from '@vueuse/core'
import {taskCommentsList, type TaskComment} from '@/client/generated'
import {queueProgressRead, type ProgressTask} from '@/helpers/projectProgress'
import {outstandingHtml} from '@/helpers/sharedOutstanding'
import {sortProgressNotes, limitProgressNotes, progressBacklinks} from '@/helpers/progressNotes'
import SubtaskOutstandingSummary from './SubtaskOutstandingSummary.vue'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
import ProgressBacklinks from '@/components/tasks/partials/ProgressBacklinks.vue'
import {taskStatusLabel} from '@/types/ITaskStatus'
import {useAuthStore} from '@/stores/auth'
const props = defineProps<{task: ProgressTask, depth: number, hasChildren?: boolean, expanded?: boolean, descendants?: ProgressTask[], progressDays?: number}>()
defineEmits<{toggle: []}>()
const element = ref<HTMLElement>()
const authStore = useAuthStore()
const history = ref<TaskComment[]>([])
const allNotes = computed(() => sortProgressNotes(history.value))
const progressBacklinkMap = computed(() => progressBacklinks(history.value))
const limitedNotes = computed(() => limitProgressNotes(allNotes.value, props.progressDays || 0))
const showAllProgress = ref(false)
const notes = computed(() => showAllProgress.value ? allNotes.value : limitedNotes.value)
const authorsByDate = computed<Record<string, string[]>>(() => {
	const result: Record<string, string[]> = {}
	for (const note of allNotes.value) {
		if (!note.author) continue
		const authors = result[note.date] ||= []
		if (!authors.includes(note.author)) authors.push(note.author)
	}
	return result
})
const showAuthors = (date: string) => {
	const authors = authorsByDate.value[date] ?? []
	return authors.length > 1 || (authors.length === 1 && authors[0].toLowerCase() !== (authStore.info?.username || '').toLowerCase())
}
const hiddenNotesCount = computed(() => allNotes.value.length - limitedNotes.value.length)
watch(() => props.progressDays, () => { showAllProgress.value = false })
// The latest daily record replaces earlier outstanding items, including clearing them.
const outstanding = computed(() => outstandingHtml(history.value))
const loading = ref(false)
const error = ref('')
let disposed = false
let requested = false
async function load() {
	if (loading.value || disposed) return
	loading.value = true; error.value = ''
	try {
		const all: TaskComment[] = []
		for (let page = 1; ; page++) {
			const result = await queueProgressRead(() => disposed ? Promise.reject(new Error('disposed')) : taskCommentsList({path: {task: props.task.id}, query: {page, per_page: 100, order_by: 'desc'}}))
			if (disposed) return
			const items = result.data.items || []
			all.push(...items)
			if (page >= (result.data.total_pages || 1) || !items.length) break
		}
		history.value = all
	} catch { if (!disposed) error.value = '进展读取失败，请重试。' }
	finally { if (!disposed) loading.value = false }
}
useIntersectionObserver(element, ([entry]) => {
	if (!entry?.isIntersecting || requested) return
	requested = true
	if (props.task.comment_count !== 0) void load()
}, {rootMargin: '200px'})
onBeforeUnmount(() => { disposed = true })
</script>

<style scoped lang="scss">
.progress-row {
 th, td {
  padding: .65rem .75rem;
  border-block-start: 1px solid var(--grey-200);
  border-inline-end: 1px solid var(--grey-200);
  vertical-align: top;
  overflow-wrap: anywhere;
  font-size: .875rem;
  text-align: start;
 }
 td:last-child { border-inline-end: 0;
 }
 th { font-weight: 600;
 }
 small {
  display: block;
  margin-block-start: .4rem;
  color: var(--grey-600);
  font-weight: 400;
 }
}
.task-name { display: flex;
 align-items: baseline;
 gap: .25rem;
 }
.toggle-spacer { inline-size: 1.5rem;
 flex-shrink: 0;
 }
.row-toggle {
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
.range-summary {
	display: inline-flex;
	border: 0;
	padding: 0;
	background: transparent;
	color: var(--grey-600);
	font-size: .75rem;
	margin-block-start: .5rem;
	cursor: pointer;
	text-decoration: underline;
	text-underline-offset: .15em;
	&:hover { color: var(--primary); }
	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: 2px;
	}
}
.empty { color: var(--grey-600);
 }
.history-entry {
 margin-block-end: .6rem;
 :deep(.readonly-rich-text), :deep(.readonly-rich-text > p:first-child) { display: inline;
 }
 time { font-weight: 600;
 }
.progress-author { margin-inline-end: .25rem; }
}
</style>
