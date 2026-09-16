<template>
	<tr
		ref="element"
		class="progress-row"
		:data-task-id="task.id"
	>
		<th
			scope="row"
			:style="{'padding-inline-start': `${Math.min(Math.max(depth - 1, 0), 6) * .8 + .75}rem`}"
		>
			<span>{{ task.title }}</span>
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
		</td>
	</tr>
</template>

<script setup lang="ts">
import {ref, computed, onBeforeUnmount} from 'vue'
import {useIntersectionObserver} from '@vueuse/core'
import {taskCommentsList, type TaskComment} from '@/client/generated'
import {queueProgressRead, type ProgressTask} from '@/helpers/projectProgress'
import {sortProgressNotes} from '@/helpers/progressNotes'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
const props = defineProps<{task: ProgressTask, depth: number}>()
const element = ref<HTMLElement>()
const history = ref<TaskComment[]>([])
const notes = computed(() => sortProgressNotes(history.value))
// The latest daily record replaces earlier outstanding items, including clearing them.
const outstanding = computed(() => notes.value.find(note => note.daily)?.outstanding || '')
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
 td:last-child { border-inline-end: 0; }
 th { font-weight: 600; }
 small {
  display: block;
  margin-block-start: .4rem;
  color: var(--grey-600);
  font-weight: 400;
 }
}
.empty { color: var(--grey-600); }
.history-entry {
 margin-block-end: .6rem;
 :deep(.readonly-rich-text), :deep(.readonly-rich-text > p:first-child) { display: inline; }
 time { font-weight: 600; }
}
</style>
