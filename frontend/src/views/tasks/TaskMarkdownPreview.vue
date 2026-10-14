<template>
	<main class="task-markdown-preview">
		<div class="preview-toolbar">
			<h1>Markdown 预览</h1>
			<TaskMarkdownExport
				v-if="title"
				:task-id="taskId"
				:title="title"
			/>
		</div>
		<p
			v-if="loading"
			role="status"
		>
			正在读取已保存的任务内容…
		</p>
		<p
			v-else-if="failure"
			role="alert"
		>
			{{ failure }}
		</p>
		<template v-else>
			<p
				v-for="warning in warnings"
				:key="warning"
				class="notification is-warning"
			>
				{{ warning }}
			</p>
			<article
				class="markdown-document"
				aria-label="任务 Markdown 内容"
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
import {ref, watch} from 'vue'
import {marked} from 'marked'
import {tasksTasktraceMarkdown} from '@/client/generated'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
import TaskMarkdownExport from '@/components/tasks/partials/TaskMarkdownExport.vue'
const props = defineProps<{taskId: number}>()
const html = ref(''), title = ref(''), failure = ref(''), loading = ref(true), warnings = ref<string[]>([])
let revision = 0
watch(() => props.taskId, async id => {
	const request = ++revision
	loading.value = true; failure.value = ''
	try {
		const {data} = await tasksTasktraceMarkdown({throwOnError: true, path: {task: id}})
		if (request !== revision) return
		const rendered = new DOMParser().parseFromString(await marked.parse(data.markdown || '', {gfm: true}), 'text/html')
		// Render GFM checkboxes as passive text before the shared HTML sanitizer removes inputs.
		for (const checkbox of rendered.querySelectorAll<HTMLInputElement>('input[type=checkbox]')) {
			checkbox.replaceWith(document.createTextNode(checkbox.checked ? '☑ ' : '☐ '))
		}
		html.value = rendered.body.innerHTML
		title.value = (data.filename || '任务.md').replace(/\.md$/i, '')
		warnings.value = data.warnings || []
	} catch {if (request === revision) failure.value = '无法读取此任务，请检查权限和连接后重试。'}
	finally {if (request === revision) loading.value = false}
}, {immediate: true})
</script>

<style scoped lang="scss">
.task-markdown-preview {
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
