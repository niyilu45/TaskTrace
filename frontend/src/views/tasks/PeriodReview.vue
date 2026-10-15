<template>
	<main class="period-review">
		<div class="preview-toolbar">
			<div>
				<h1>周期回顾</h1><p v-if="report">
					共 {{ report.task_count }} 个任务 · {{ report.change_count }} 条变化
				</p>
			</div>
			<XButton
				v-if="report"
				variant="secondary"
				icon="download"
				:disabled="saving"
				@click="save"
			>
				导出 Markdown
			</XButton>
		</div>
		<p
			v-if="loading"
			role="status"
		>
			正在汇总所选项目的进展和遗留事项变化…
		</p>
		<div
			v-else-if="failure"
			role="alert"
		>
			<p>{{ failure }}</p><XButton
				variant="secondary"
				@click="load"
			>
				重试
			</XButton>
		</div>
		<template v-else-if="report">
			<p
				v-for="warning in report.warnings"
				:key="warning"
				class="notification is-warning"
			>
				{{ warning }}
			</p>
			<p
				v-if="saveMessage"
				role="status"
			>
				{{ saveMessage }}
			</p>
			<article
				class="markdown-document"
				aria-label="周期回顾 Markdown 内容"
			>
				<ReadonlyRichText
					:html="html"
					:deduplicate-images="false"
				/>
			</article>
		</template>
	</main>
</template>

<script setup lang="ts">
import {ref, watch, onBeforeUnmount} from 'vue'
import {useRoute} from 'vue-router'
import {marked} from 'marked'
import {tasktracePeriodReview, type TaskTracePeriodReview} from '@/client/generated'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
import XButton from '@/components/input/Button.vue'
import {saveMarkdownDocument} from '@/helpers/taskMarkdown'
const route = useRoute()
const loading = ref(false), saving = ref(false), failure = ref(''), saveMessage = ref(''), html = ref('')
const report = ref<TaskTracePeriodReview>()
let controller: AbortController | undefined
async function load() {
	controller?.abort(); const request = controller = new AbortController()
	loading.value = true; failure.value = ''; report.value = undefined
	try {
		const projects = String(route.query.projects || '').split(',').map(Number)
		const from = String(route.query.from || ''), until = String(route.query.until || '')
		if (projects.some(id => !Number.isSafeInteger(id) || id < 1) || !from || !until) throw new Error('范围无效')
		const {data} = await tasktracePeriodReview({throwOnError: true, query: {projects, from, until}, querySerializer: {array: {explode: false, style: 'form'}}, signal: request.signal})
		const rendered = new DOMParser().parseFromString(await marked.parse(data.markdown || '', {gfm: true}), 'text/html')
		// Preserve GFM task states before the shared sanitizer removes form inputs.
		for (const checkbox of rendered.querySelectorAll<HTMLInputElement>('input[type=checkbox]')) {
			checkbox.replaceWith(document.createTextNode(checkbox.checked ? '☑ ' : '☐ '))
		}
		if (controller !== request) return
		report.value = data; html.value = rendered.body.innerHTML
	} catch {
		if (!request.signal.aborted) failure.value = '无法生成回顾。请检查连接和项目访问权限，或回到概览重新选择范围。'
	} finally {if (controller === request) loading.value = false}
}
async function save() {
	if (!report.value || saving.value) return
	saving.value = true; saveMessage.value = ''
	try {await saveMarkdownDocument(report.value.markdown || '', report.value.filename || '周期回顾.md'); saveMessage.value = 'Markdown 文件已保存。'}
	catch (error) {saveMessage.value = error instanceof Error && error.name === 'AbortError' ? '已取消导出。' : '导出失败，请重试。'}
	finally {saving.value = false}
}
watch(() => route.fullPath, load, {immediate: true})
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped lang="scss">

.period-review {
	max-inline-size: 64rem;
	margin-inline: auto;
	padding: 1.5rem;
}
.preview-toolbar {
	display: flex;
	align-items: center;
	justify-content: space-between;
	flex-wrap: wrap;
	gap: 1rem;
	margin-block-end: 1.5rem;
}
.preview-toolbar h1 {
	margin: 0;
	font-size: 1.25rem;
}
.markdown-document {
	background: var(--white);
	color: var(--grey-900);
	padding: clamp(1rem, 4vw, 3rem);
	border: 1px solid var(--grey-200);
	border-radius: .5rem;
	line-height: 1.6;
}
.markdown-document :deep(h1), .markdown-document :deep(h2) {
	padding-block-end: .3em;
	border-block-end: 1px solid var(--grey-200);
}
.markdown-document :deep(h1) {
	font-size: 2em;
}
.markdown-document :deep(h2) {
	font-size: 1.5em;
}
.markdown-document :deep(h3) {
	font-size: 1.25em;
}
.markdown-document :deep(h1), .markdown-document :deep(h2), .markdown-document :deep(h3) {
	margin-block: 1.5em .65em;
	font-weight: 600;
}
.markdown-document :deep(p), .markdown-document :deep(ul), .markdown-document :deep(ol) {
	margin-block: 0 1em;
}
.markdown-document :deep(ul), .markdown-document :deep(ol) {
	padding-inline-start: 2em;
}
.markdown-document :deep(ul) {
	list-style: disc;
}
.markdown-document :deep(ol) {
	list-style: decimal;
}
.markdown-document :deep(pre) {
	padding: 1rem;
	background: var(--grey-100);
	border-radius: .375rem;
}
.markdown-document :deep(td), .markdown-document :deep(th) {
	border: 1px solid var(--grey-300);
	padding: .4em .8em;
}
</style>
