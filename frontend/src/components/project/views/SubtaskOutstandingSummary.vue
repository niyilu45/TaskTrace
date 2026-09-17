<template>
	<section
		ref="element"
		class="subtask-outstanding"
		aria-label="子任务遗留事项汇总"
	>
		<strong>子任务汇总（含下级）</strong>
		<p
			v-if="loading"
			role="status"
		>
			正在汇总遗留事项…
		</p>
		<p
			v-if="failed.length"
			role="alert"
		>
			{{ failed.length }} 个子任务读取失败，汇总尚不完整。
			<XButton
				variant="secondary"
				:disabled="loading"
				@click="load"
			>
				重试汇总
			</XButton>
		</p>
		<p
			v-if="!loading && !failed.length && !entries.length"
			class="empty"
		>
			子任务暂无遗留事项
		</p>
		<div
			v-for="entry in entries"
			:key="entry.task.id"
			class="outstanding-source"
		>
			<span>{{ entry.task.title }}{{ entry.task.done ? '（已完成）' : '' }}：</span>
			<ReadonlyRichText :html="entry.html" />
		</div>
	</section>
</template>

<script setup lang="ts">
import {ref, onBeforeUnmount} from 'vue'
import {useIntersectionObserver} from '@vueuse/core'
import {taskCommentsList, type TaskComment} from '@/client/generated'
import {queueProgressRead, type ProgressTask} from '@/helpers/projectProgress'
import {outstandingHtml} from '@/helpers/sharedOutstanding'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
const props = defineProps<{tasks: ProgressTask[]}>()
const element = ref<HTMLElement>()
const loading = ref(false)
const entries = ref<{task: ProgressTask, html: string}[]>([])
const failed = ref<number[]>([])
let disposed = false
let requested = false
async function load() {
	if (loading.value || disposed) return
	loading.value = true
	const results = await Promise.all(props.tasks.map(async task => {
		try {
			const history: TaskComment[] = []
			if (task.comment_count !== 0) {
				for (let page = 1; ; page++) {
					const result = await queueProgressRead(() => disposed ? Promise.reject(new Error('disposed')) : taskCommentsList({path: {task: task.id}, query: {page, per_page: 100, order_by: 'desc'}}))
					if (disposed) return {task, html: '', failed: false}
					const items = result.data.items || []
					history.push(...items)
					if (page >= (result.data.total_pages || 1) || !items.length) break
				}
			}
			const html = outstandingHtml(history)
			const doc = new DOMParser().parseFromString(html, 'text/html')
			const hasContent = !!doc.body.textContent?.trim() || !!doc.body.querySelector('img')
			return {task, html: hasContent ? html : '', failed: false}
		} catch { return {task, html: '', failed: true} }
	}))
	if (disposed) return
	entries.value = results.filter(result => !!result.html)
	failed.value = results.filter(result => result.failed).map(result => result.task.id)
	loading.value = false
}
useIntersectionObserver(element, ([entry]) => {
	if (!entry?.isIntersecting || requested) return
	requested = true
	void load()
}, {rootMargin: '200px'})
onBeforeUnmount(() => { disposed = true })
</script>

<style scoped lang="scss">
.subtask-outstanding {
    margin-block-start: .75rem;
}
.outstanding-source {
    margin-block-start: .5rem;
    > span { font-weight: 600; }
}
.empty { color: var(--grey-600); }
</style>
