<template>
	<div
		class="readonly-rich-text"
		v-html="rendered"
	/>
</template>

<script setup lang="ts">
import {ref, watch, onBeforeUnmount} from 'vue'
import DOMPurify from 'dompurify'
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'
const props = defineProps<{html?: string}>()
const rendered = ref('')
let version = 0
watch(() => props.html, async value => {
	const current = ++version
	const doc = new DOMParser().parseFromString(DOMPurify.sanitize(value || '', {FORBID_TAGS: ['input', 'button', 'form', 'textarea', 'select'], FORBID_ATTR: ['contenteditable', 'autofocus']}), 'text/html')
	for (const anchor of doc.querySelectorAll('a')) { anchor.removeAttribute('target'); anchor.setAttribute('rel', 'noopener noreferrer') }
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
	overflow-wrap: anywhere;
	:deep(img) {
	max-inline-size: 100%;
	max-block-size: 24rem;
	object-fit: contain;
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
