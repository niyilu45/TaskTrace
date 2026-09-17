<template>
	<form
		class="daily-progress"
		@submit.prevent="save(false)"
		@paste="pasteImages"
		@keydown.ctrl.enter.prevent="save(false)"
	>
		<h3 class="task-section-title">
			记录每日进展
		</h3>
		<AutoSaveSettings />
		<button
			v-if="restoring"
			type="button"
			class="button"
			:disabled="saving"
			@click="switchDate(date, true)"
		>
			重新读取当天进展
		</button>
		<label :for="`progress-date-${taskId}`">记录日期</label>
		<input
			:id="`progress-date-${taskId}`"
			:value="date"
			class="input"
			type="date"
			required
			:disabled="saving"
			@change="switchDate(($event.target as HTMLInputElement).value)"
		>
		<label :for="`progress-text-${taskId}`">今日进展</label>
		<textarea
			:id="`progress-text-${taskId}`"
			v-model="progress"
			class="textarea"
			rows="3"
			placeholder="今天完成了什么？"
			:disabled="saving || restoring"
		/>
		<SharedOutstanding
			:task-id="taskId"
			:disabled="saving"
			@saved="emit('saved')"
		/>
		<ReadonlyRichText
			v-if="existingImages"
			:html="existingImages"
		/>
		<p>截图或复制图片后，在这里按 Ctrl+V，可连续粘贴多张图片。图片会随进展自动保存，也可点击“保存进展”。</p>
		<div
			v-if="images.length"
			class="progress-images"
		>
			<figure
				v-for="(picture, index) in images"
				:key="picture.preview"
			>
				<img
					:src="picture.preview"
					:alt="`待保存图片 ${index + 1}`"
				>
				<button
					type="button"
					class="button"
					:disabled="saving || restoring"
					@click="removeImage(index)"
				>
					移除图片 {{ index + 1 }}
				</button>
			</figure>
		</div>
		<div class="daily-progress-actions">
			<button
				class="button is-primary"
				type="submit"
				:disabled="saving || restoring || (!progress.trim() && images.length === 0 && !autoCommentId)"
			>
				{{ saving ? '正在保存…' : '保存进展' }}
			</button>
			<span role="status">{{ message || 'Ctrl + Enter 快速保存，历史记录保留在下方。' }}</span>
		</div>
	</form>
</template>

<script setup lang="ts">
import {ref, watch, onBeforeUnmount} from 'vue'
import {taskCommentsCreate, taskCommentsUpdate, taskAttachmentsUpload} from '@/client/generated'
import AutoSaveSettings from './AutoSaveSettings.vue'
import SharedOutstanding from './SharedOutstanding.vue'
import ReadonlyRichText from './ReadonlyRichText.vue'
import {readTaskHistory, sharedOutstanding, changeOutstanding} from '@/helpers/sharedOutstanding'
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'
import {mergedDay} from '@/helpers/progressNotes'
import {useAutoSave} from '@/helpers/autoSave'
const props = defineProps<{taskId: number}>()
const emit = defineEmits<{saved: []}>()
const date = ref('')
const progress = ref('')
const images = ref<{file?: File, preview: string, attachmentId?: number}[]>([])
const existingImages = ref('')
const originalHtml = ref('')
const originalText = ref('')
const mergedIds = ref<number[]>([])
const autoCommentId = ref<number>()
const saving = ref(false)
const restoring = ref(true)
const message = ref('')
const lastSaved = ref('')
let version = 0
const snapshot = () => JSON.stringify([date.value, progress.value, images.value.map(image => image.attachmentId || image.preview)])
const drafts = new Map<string, {text: string, images: typeof images.value}>()
function stash() {
	if (restoring.value || !date.value || snapshot() === lastSaved.value) return
	drafts.set(`${props.taskId}:${date.value}`, {text: progress.value, images: [...images.value]})
	try { localStorage.setItem(`tasktrace-day-draft-${props.taskId}-${date.value}`, JSON.stringify({progress: progress.value, attachments: images.value.map(image => image.attachmentId).filter(Boolean)})) } catch { /* Server save remains available. */ }
}
async function switchDate(value: string, initial = false) {
	if (saving.value || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return
	if (!initial) stash()
	const request = ++version
	const taskId = props.taskId
	restoring.value = true; date.value = value; message.value = ''
	let loaded = false
	try {
		const history = await readTaskHistory(taskId)
		if (request !== version || taskId !== props.taskId) return
		const selected = mergedDay(history, value)
		date.value = value; autoCommentId.value = selected.id; mergedIds.value = selected.mergedIds
		originalHtml.value = selected.html; originalText.value = selected.text; existingImages.value = selected.images
		progress.value = selected.text; images.value = []
		let draft = drafts.get(`${taskId}:${value}`)
		if (!draft) {
			try {
				let saved = JSON.parse(localStorage.getItem(`tasktrace-day-draft-${taskId}-${value}`) || 'null')
				const legacy = JSON.parse(localStorage.getItem(`tasktrace-progress-draft-${taskId}`) || 'null')
				if (!saved && legacy?.date === value) saved = legacy
				if (saved && typeof saved.progress === 'string') {
					const pictures: typeof images.value = []
					for (const id of saved.attachments || []) if (Number.isInteger(id) && id > 0) pictures.push({attachmentId: id, preview: await fetchAttachmentBlobUrl({taskId, id})})
					draft = {text: saved.progress, images: pictures}
				}
			} catch { message.value = '草稿恢复失败，已保留服务器内容。' }
		}
		if (request !== version || taskId !== props.taskId) return
		lastSaved.value = snapshot()
		if (draft) { progress.value = draft.text; images.value = draft.images }
		loaded = true
		message.value = selected.id ? '已载入当天进展；同日记录合并编辑，保存会更新当天内容。' : '此日期尚无进展。'
	} catch { message.value = '历史读取失败，已暂停保存，请重新选择日期重试。'; return }
	finally { if (request === version) restoring.value = !loaded }
}
watch(() => props.taskId, async () => {
	const now = new Date()
	const today = `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,'0')}-${String(now.getDate()).padStart(2,'0')}`
	await switchDate(today, true)
}, {immediate: true})
watch([progress, images], stash, {deep: true})
function pasteImages(event: ClipboardEvent) {
	const files = Array.from(event.clipboardData?.items || []).filter(item => item.kind === 'file' && item.type.startsWith('image/')).map(item => item.getAsFile()).filter((file): file is File => !!file)
	if (!files.length) return
	event.preventDefault()
	if (saving.value || restoring.value) return
	for (const file of files) images.value.push({file, preview: URL.createObjectURL(file)})
	message.value = `已粘贴 ${images.value.length} 张图片。`
}
function removeImage(index: number) { const [picture] = images.value.splice(index, 1); if (picture?.file) URL.revokeObjectURL(picture.preview) }
function html(value: string) { return value.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\r?\n/g,'<br>') }
async function save(automatic = false) {
	if (restoring.value || saving.value || (!progress.value.trim() && images.value.length === 0 && !autoCommentId.value)) return
	saving.value = true
	const taskId = props.taskId
	try {
		const latestHistory = await readTaskHistory(taskId)
		if (!sharedOutstanding(latestHistory).id) await changeOutstanding(taskId, items => items)
		let body = progress.value === originalText.value ? originalHtml.value : `<p>${html(progress.value.trim())}</p>${existingImages.value}`
		for (const picture of images.value) {
			if (!picture.attachmentId) {
				const result = await taskAttachmentsUpload({path: {task: taskId}, body: {files: [picture.file!]}})
				if (!result.data.success?.[0]?.id || result.data.errors?.length) throw new Error('Upload failed')
				picture.attachmentId = result.data.success[0].id
			}
			body += `<p><img src="/api/v1/tasks/${taskId}/attachments/${picture.attachmentId}" alt="进展图片"></p>`
		}
		const comment = `<h3 data-tasktrace-merged="${mergedIds.value.join(',')}">每日进展 · ${date.value}</h3>${body}`
		if (snapshot() !== lastSaved.value || mergedIds.value.length) {
			if (autoCommentId.value) await taskCommentsUpdate({path: {task: taskId, commentid: autoCommentId.value}, body: {comment}})
			else autoCommentId.value = (await taskCommentsCreate({path: {task: taskId}, body: {comment}})).data.id
		}
		if (taskId !== props.taskId) return
		originalHtml.value = body; originalText.value = progress.value
		existingImages.value = Array.from(new DOMParser().parseFromString(body,'text/html').querySelectorAll('img')).map(img => img.outerHTML).join('')
		images.value.filter(picture => picture.file).forEach(picture => URL.revokeObjectURL(picture.preview)); images.value = []; lastSaved.value = snapshot(); drafts.delete(`${taskId}:${date.value}`)
		try { localStorage.removeItem(`tasktrace-progress-draft-${taskId}`); localStorage.removeItem(`tasktrace-day-draft-${taskId}-${date.value}`) } catch { /* Optional draft. */ }
		message.value = automatic ? '已自动保存当天进展。' : '当天进展已保存，可继续修改。'; emit('saved')
	} catch { message.value = '保存失败，内容已保留，请重试。' }
	finally { saving.value = false }
}
useAutoSave(async () => { if (!restoring.value && snapshot() !== lastSaved.value) await save(true) })
onBeforeUnmount(() => { ++version; stash(); const urls = new Set([...images.value, ...[...drafts.values()].flatMap(draft => draft.images)].filter(image => image.file).map(image => image.preview)); urls.forEach(url => URL.revokeObjectURL(url)) })
</script>

<style scoped lang="scss">
.daily-progress {
	display: grid;
	gap: .65rem;
	margin-block-end: 1.5rem;
	padding-block-end: 1.5rem;
	border-block-end: 1px solid var(--grey-200);

	input { max-inline-size: 12rem; }
	label { font-weight: 600; }
}
.progress-images {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    figure {
        margin: 0;
        display: grid;
        gap: .5rem;
    }
    img {
        inline-size: 10rem;
        block-size: 7rem;
        object-fit: contain;
    }
}
.daily-progress-actions {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: .75rem;
	span { font-size: .875rem; }
}
</style>
