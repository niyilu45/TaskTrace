<template>
	<details
		ref="element"
		class="progress-row"
		:data-task-id="task.id"
		@toggle="toggle"
	>
		<summary :style="{'padding-inline-start': `${Math.min(depth, 6) * .8 + .6}rem`}">
			<span
				class="task-state"
				:class="{'is-done': task.done}"
			>{{ task.done ? '已完成' : '进行中' }}</span>
			<span class="task-name">{{ task.title }}<small v-if="depth">子任务</small></span>
			<span class="latest-summary">{{ error || (latest ? plain(latest.comment) : (loading ? '加载进展…' : '暂无进展记录')) }}</span>
			<time>{{ stamp(latest?.created || task.updated) }}</time>
		</summary>
		<div
			v-if="opened"
			class="progress-detail"
		>
			<h4>事项说明</h4>
			<ReadonlyRichText
				v-if="task.description"
				:html="task.description"
			/>
			<p v-else>
				暂无说明
			</p>
			<h4>进展记录 <small>最新在前</small></h4>
			<p
				v-if="error"
				role="alert"
			>
				{{ error }} <XButton
					variant="secondary"
					@click="loadHistory(true)"
				>
					重试
				</XButton>
			</p>
			<p v-if="!history.length && !historyLoading && !error">
				尚未记录进展，可进入编辑模式添加。
			</p>
			<article
				v-for="note in history"
				:key="note.id"
				class="history-entry"
			>
				<time>{{ stamp(note.created, true) }}</time>
				<ReadonlyRichText :html="note.comment" />
			</article>
			<p
				v-if="historyLoading"
				role="status"
			>
				正在读取记录…
			</p>
			<XButton
				v-if="history.length < total && !historyLoading"
				variant="secondary"
				@click="loadHistory(false)"
			>
				更早的记录
			</XButton>
		</div>
	</details>
</template>

<script setup lang="ts">
import {ref, onBeforeUnmount} from 'vue'
import {useIntersectionObserver} from '@vueuse/core'
import {taskCommentsList, type TaskComment} from '@/client/generated'
import {queueProgressRead, type ProgressTask} from '@/helpers/projectProgress'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
const props = defineProps<{task: ProgressTask, depth: number}>()
const element = ref<HTMLElement>()
const opened = ref(false)
const latest = ref<TaskComment>()
const loading = ref(false)
const error = ref('')
const history = ref<TaskComment[]>([])
const historyLoading = ref(false)
const total = ref(0)
let page = 0
let disposed = false
let requested = false
function plain(html = '') { return new DOMParser().parseFromString(html, 'text/html').body.textContent?.trim() || '图片进展' }
function stamp(value?: string, full = false) {
	if (!value) return ''
	const date = new Date(value)
	return Number.isNaN(+date) ? '' : full ? date.toLocaleString('zh-CN') : date.toLocaleDateString('zh-CN')
}
useIntersectionObserver(element, ([entry]) => {
	if (!entry?.isIntersecting || requested) return
	requested = true
	if (!props.task.comment_count) return
	loading.value = true
	void queueProgressRead(() => taskCommentsList({path: {task: props.task.id}, query: {per_page: 1, order_by: 'desc'}}))
		.then(result => { if (!disposed) latest.value = result.data.items?.[0] })
		.catch(() => { if (!disposed) error.value = '进展加载失败，展开重试' })
		.finally(() => { if (!disposed) loading.value = false })
}, {rootMargin: '200px'})
async function loadHistory(reset: boolean) {
	if (historyLoading.value) return
	historyLoading.value = true; error.value = ''
	const next = reset ? 1 : page + 1
	try {
		const result = await queueProgressRead(() => taskCommentsList({path: {task: props.task.id}, query: {page: next, per_page: 20, order_by: 'desc'}}))
		if (disposed) return
		history.value = reset ? result.data.items || [] : [...history.value, ...(result.data.items || [])]
		total.value = result.data.total || 0; page = next
		latest.value = history.value[0]
	} catch { error.value = '进展加载失败，请重试。' }
	finally { historyLoading.value = false }
}
function toggle(event: Event) {
	opened.value = (event.target as HTMLDetailsElement).open
	if (opened.value && page === 0) void loadHistory(true)
}
onBeforeUnmount(() => { disposed = true })
</script>

<style scoped lang="scss">
.progress-row {
	border-block-start: 1px solid var(--grey-200);
	summary {
	display: grid;
	grid-template-columns: 4.5rem minmax(9rem, 1fr) minmax(8rem, 1.2fr) 6rem;
	gap: .65rem;
	align-items: center;
	padding: .55rem .75rem;
	cursor: pointer;
	font-size: .875rem;
	}
	summary:hover {
	background: var(--grey-100);
	}
	summary:focus-visible {
	outline: 2px solid var(--primary);
	outline-offset: -2px;
	}
	small {
	display: inline-block;
	margin-inline-start: .5rem;
	color: var(--grey-600);
	font-size: .75rem;
	}
	time {
	font-size: .75rem;
	color: var(--grey-600);
	}
}
.task-name {
	font-weight: 600;
	overflow-wrap: anywhere;
	}
.task-state {
	font-size: .75rem;
	color: var(--grey-700);
	}
.is-done {
	color: var(--success);
	}
.latest-summary {
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	color: var(--grey-600);
	}
.progress-detail {
	padding: .75rem 1.25rem 1.25rem;
	font-size: .875rem;
	}
.history-entry {
	padding-block: .75rem;
	border-block-end: 1px solid var(--grey-200);
	}
@media (width <= 700px) {
	.progress-row summary {
	grid-template-columns: 4rem minmax(0, 1fr);
	}
	.latest-summary {
	grid-column: 2;
	}
	.progress-row summary > time {
	display: none;
	}
}
</style>
