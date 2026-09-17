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
			<small>{{ task.done ? '已完成' : '未完成' }}<template v-if="depth > 1"> · 下级子任务</template></small>
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
				<time>{{ note.date }}：</time><ReadonlyRichText :html="note.progress" />
			</div>
			<p
				v-if="hiddenNotesCount > 0"
				class="range-summary"
			>
				已隐藏 {{ hiddenNotesCount }} 条较早进展
			</p>
		</td>
	</tr>
</template>

<script setup lang="ts">
import {ref, computed, onBeforeUnmount} from 'vue'
import {useIntersectionObserver} from '@vueuse/core'
import {taskCommentsList, type TaskComment} from '@/client/generated'
import {queueProgressRead, type ProgressTask} from '@/helpers/projectProgress'
import {outstandingHtml} from '@/helpers/sharedOutstanding'
import {sortProgressNotes, limitProgressNotes} from '@/helpers/progressNotes'
import SubtaskOutstandingSummary from './SubtaskOutstandingSummary.vue'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
const props = defineProps<{task: ProgressTask, depth: number, hasChildren?: boolean, expanded?: boolean, descendants?: ProgressTask[], progressDays?: number}>()
defineEmits<{toggle: []}>()
const element = ref<HTMLElement>()
const history = ref<TaskComment[]>([])
const allNotes = computed(() => sortProgressNotes(history.value))
const notes = computed(() => limitProgressNotes(allNotes.value, props.progressDays || 0))
const hiddenNotesCount = computed(() => allNotes.value.length - notes.value.length)
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
 color: var(--grey-600);
 font-size: .75rem;
 margin-block-start: .5rem;
}
.empty { color: var(--grey-600);
 }
.history-entry {
 margin-block-end: .6rem;
 :deep(.readonly-rich-text), :deep(.readonly-rich-text > p:first-child) { display: inline;
 }
 time { font-weight: 600;
 }
}
</style>
