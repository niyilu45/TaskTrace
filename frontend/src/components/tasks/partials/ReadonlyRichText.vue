<template>
	<div
		class="readonly-rich-text"
		@click="handleContentClick"
		v-html="rendered"
	/>
	<ImageLightbox
		v-if="lightboxSrc"
		:blob-url="lightboxSrc"
		:alt="lightboxAlt"
		@close="lightboxSrc = null"
	/>
</template>

<script setup lang="ts">
import {ref, watch, onBeforeUnmount} from 'vue'
import DOMPurify from 'dompurify'
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'
import {deduplicateHtmlImages} from '@/helpers/tasktraceImages'
import ImageLightbox from '@/components/misc/ImageLightbox.vue'

const props = defineProps<{html?: string}>()
const rendered = ref('')
const lightboxSrc = ref<string | null>(null)
const lightboxAlt = ref('')

function handleContentClick(event: MouseEvent) {
	const target = event.target
	if (!(target instanceof HTMLImageElement) || !target.src.startsWith('blob:')) return
	lightboxSrc.value = target.src
	lightboxAlt.value = target.alt || '每日进展图片'
}

let version = 0
watch(() => props.html, async value => {
	const current = ++version
	const doc = new DOMParser().parseFromString(DOMPurify.sanitize(deduplicateHtmlImages(value || ''), {FORBID_TAGS: ['input', 'button', 'form', 'textarea', 'select'], FORBID_ATTR: ['contenteditable', 'autofocus']}), 'text/html')
	for (const anchor of doc.querySelectorAll('a')) {
		anchor.removeAttribute('target')
		anchor.setAttribute('rel', 'noopener noreferrer')
		if (anchor.closest('blockquote[data-tasktrace-reference="1"]') && /^\/tasks\/\d+#comment-\d+$/.test(anchor.getAttribute('href') || '')) anchor.setAttribute('target', '_blank')
	}
	for (const quote of Array.from(doc.body.querySelectorAll('blockquote[data-tasktrace-reference="1"]'))) {
		const date = quote.getAttribute('data-date') || '历史日期'
		const details = doc.createElement('details')
		details.className = 'progress-reference-details'
		const summary = doc.createElement('summary')
		const closed = doc.createElement('span')
		closed.className = 'reference-summary-closed'
		closed.textContent = `展开引用的历史进展（${date}）`
		const opened = doc.createElement('span')
		opened.className = 'reference-summary-open'
		opened.textContent = `收起引用的历史进展（${date}）`
		summary.append(closed, opened)
		quote.replaceWith(details)
		details.append(summary, quote)
	}
	const images = Array.from(doc.querySelectorAll('img'))
	await Promise.all(images.map(async image => {
		const source = image.getAttribute('data-src') || image.getAttribute('src') || ''
		let parsed: URL
		try { parsed = new URL(source, window.location.origin) } catch { image.remove(); return }
		const api = new URL(window.API_URL, window.location.origin)
		const match = parsed.pathname.match(/\/tasks\/(\d+)\/attachments\/(\d+)$/)
		if (match && (parsed.origin === api.origin || (parsed.hostname === '127.0.0.1' && location.hostname === '127.0.0.1'))) {
			try { image.src = await fetchAttachmentBlobUrl({taskId: Number(match[1]), id: Number(match[2])}) }
			catch { image.removeAttribute('src'); image.alt = '图片加载失败，请刷新重试' }
		}
		image.setAttribute('loading', 'lazy')
	}))
	if (version === current) rendered.value = doc.body.innerHTML
}, {immediate: true})
onBeforeUnmount(() => version++)
</script>

<style scoped lang="scss">
.readonly-rich-text {
	min-inline-size: 0;
	max-inline-size: 100%;
	overflow-wrap: anywhere;
	word-break: break-word;
	:deep(a) {
		overflow-wrap: anywhere;
		word-break: break-all;
	}
	:deep(pre) {
		max-inline-size: 100%;
		overflow-x: auto;
		white-space: pre-wrap;
	}
	:deep(table) {
		display: block;
		max-inline-size: 100%;
		overflow-x: auto;
	}
	:deep(blockquote[data-tasktrace-reference="1"]) {
		margin: .65rem 0;
		padding: .4rem .75rem;
		border-inline-start: 3px solid var(--grey-300);
		background: var(--grey-50);
		font-size: .9em;
	}
	:deep(.progress-reference-details) {
		margin-block: .5rem;

		summary {
			color: var(--primary);
			cursor: pointer;
			text-decoration: underline;
			text-underline-offset: .15em;
		}
		.reference-summary-open { display: none; }
		&[open] {
			.reference-summary-closed { display: none; }
			.reference-summary-open { display: inline; }
		}
	}
	:deep(img) {
		display: block;
		max-inline-size: 100%;
		max-block-size: 24rem;
		object-fit: contain;
		cursor: zoom-in;
	}
	:deep(p) {
		margin-block: .4rem;
	}
	:deep(h3) {
		font-size: 1rem;
		margin-block: .75rem .4rem;
	}
}
</style>
