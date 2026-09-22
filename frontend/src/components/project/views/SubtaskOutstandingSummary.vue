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
			role="button"
			tabindex="0"
			aria-label="打开所属任务编辑卡片"
			@click="$emit('edit', entry.task.id)"
			@keydown.enter.prevent="$emit('edit', entry.task.id)"
			@keydown.space.prevent="$emit('edit', entry.task.id)"
		>
			<span>{{ entry.task.title }}{{ entry.task.done ? '（已完成）' : '' }}：</span>
			<ReadonlyRichText :html="entry.html" />
		</div>
	</section>
</template>

<script setup lang="ts">
import {ref, onBeforeUnmount, inject, watch} from 'vue'
import equal from 'fast-deep-equal'
import {useIntersectionObserver} from '@vueuse/core'
import {createProjectProgressHistory, projectProgressHistoryKey, isProgressReadCancelled} from '@/helpers/projectProgressHistory'
import {type ProgressTask} from '@/helpers/projectProgress'
import {outstandingHtml} from '@/helpers/sharedOutstanding'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
const props = defineProps<{tasks: ProgressTask[], refreshRevision?: number}>()
defineEmits<{edit: [taskId: number]}>()
const sharedProgressHistory = inject(projectProgressHistoryKey, null)
const progressHistory = sharedProgressHistory || createProjectProgressHistory()
const element = ref<HTMLElement>()
const loading = ref(false)
const entries = ref<{task: ProgressTask, html: string}[]>([])
const failed = ref<number[]>([])
let disposed = false
let requested = false
let loadVersion = 0
async function load() {
	if (disposed) return
	const version = ++loadVersion
	loading.value = true
	let cancelled = false
	const results = await Promise.all(props.tasks.map(async task => {
		try {
			const history = task.comment_count === 0 ? [] : await progressHistory.read(task.id)
			if (disposed || version !== loadVersion) return {task, html: '', failed: false}
			const html = outstandingHtml(history)
			const doc = new DOMParser().parseFromString(html, 'text/html')
			const hasContent = !!doc.body.textContent?.trim() || !!doc.body.querySelector('img')
			return {task, html: hasContent ? html : '', failed: false}
		} catch (failure) {
			if (isProgressReadCancelled(failure)) cancelled = true
			return {task, html: '', failed: !isProgressReadCancelled(failure)}
		}
	}))
	if (disposed || version !== loadVersion) return
	loading.value = false
	if (cancelled) return
	const next = results.filter(result => !!result.html)
	if (!equal(entries.value, next)) entries.value = next
	failed.value = results.filter(result => result.failed).map(result => result.task.id)
}
watch(() => [props.tasks.map(task => task.id).join(','), props.refreshRevision], () => {
	if (!sharedProgressHistory) progressHistory.clear()
	if (requested) void load()
})
useIntersectionObserver(element, ([entry]) => {
	if (!entry?.isIntersecting || requested) return
	requested = true
	void load()
}, {rootMargin: '200px'})
onBeforeUnmount(() => {
	disposed = true
	if (!sharedProgressHistory) progressHistory.clear()
})
</script>

<style scoped lang="scss">
.subtask-outstanding {
    margin-block-start: .75rem;
}
.outstanding-source {
    margin-block-start: .5rem;
	cursor: pointer;
	border-radius: .2rem;
    > span { font-weight: 600; }
	&:hover { color: var(--primary); }
	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: 2px;
	}
}
.empty { color: var(--grey-600); }
</style>
