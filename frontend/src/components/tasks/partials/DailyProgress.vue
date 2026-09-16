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
		<label :for="`progress-date-${taskId}`">记录日期</label>
		<input
			:id="`progress-date-${taskId}`"
			v-model="date"
			class="input"
			type="date"
			required
			:disabled="saving"
		>
		<label :for="`progress-text-${taskId}`">今日进展</label>
		<textarea
			:id="`progress-text-${taskId}`"
			v-model="progress"
			class="textarea"
			rows="3"
			placeholder="今天完成了什么？"
			:disabled="saving"
		/>
		<label :for="`progress-next-${taskId}`">遗留问题 / 下一步（选填）</label>
		<textarea
			:id="`progress-next-${taskId}`"
			v-model="next"
			class="textarea"
			rows="2"
			placeholder="还有什么需要继续跟进？"
			:disabled="saving"
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
					:disabled="saving"
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
				:disabled="saving || (!progress.trim() && images.length === 0)"
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
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'
import {useAutoSave} from '@/helpers/autoSave'

const props = defineProps<{taskId: number}>()
const emit = defineEmits<{saved: []}>()
const autoCommentId = ref<number | undefined>()
const lastSaved = ref('')
const restoring = ref(true)
const snapshot = () => JSON.stringify([date.value, progress.value, next.value, images.value.map(picture => picture.attachmentId || picture.preview)])
useAutoSave(async () => { if (!restoring.value && snapshot() !== lastSaved.value) await save(true) })
const images = ref<{file?: File, preview: string, attachmentId?: number}[]>([])
function pasteImages(event: ClipboardEvent) {
	const files = Array.from(event.clipboardData?.items || []).filter(item => item.kind === 'file' && item.type.startsWith('image/')).map(item => item.getAsFile()).filter((file): file is File => !!file)
	if (!files.length) return
	event.preventDefault()
	if (saving.value) return
	for (const file of files) images.value.push({file, preview: URL.createObjectURL(file)})
	message.value = `已粘贴 ${images.value.length} 张图片，保存进展后上传。`
}
function removeImage(index: number) {
	if (images.value[index].file) URL.revokeObjectURL(images.value[index].preview)
	images.value.splice(index, 1)
}
onBeforeUnmount(() => images.value.forEach(picture => { if (picture.file) URL.revokeObjectURL(picture.preview) }))
const date = ref('')
const progress = ref('')
const next = ref('')
const saving = ref(false)
const message = ref('')
const key = () => `tasktrace-progress-draft-${props.taskId}`
watch(() => props.taskId, async () => {
	const today = new Date()
	date.value = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
	progress.value = ''; next.value = ''; message.value = ''
	try {
		const draft = JSON.parse(localStorage.getItem(key()) || 'null')
		if (draft && typeof draft.progress === 'string' && typeof draft.next === 'string') {
			progress.value = draft.progress; next.value = draft.next
			if (Number.isInteger(draft.commentId) && draft.commentId > 0) autoCommentId.value = draft.commentId
			if (/^\d{4}-\d{2}-\d{2}$/.test(draft.date)) date.value = draft.date
			if (Array.isArray(draft.attachments)) {
				for (const id of draft.attachments) {
					if (Number.isInteger(id) && id > 0) images.value.push({attachmentId: id, preview: await fetchAttachmentBlobUrl({taskId: props.taskId, id})})
				}
			}
			if (typeof draft.lastSaved === 'string') lastSaved.value = draft.lastSaved
		}
	} catch { message.value = '草稿图片加载失败，请刷新后重试，自动保存已暂停。'; return } finally { /* Keep restoration separate from edits. */ }
	restoring.value = false
}, {immediate: true})
watch([date, progress, next, autoCommentId, lastSaved], () => {
	if (restoring.value) return
	try { localStorage.setItem(key(), JSON.stringify({date: date.value, progress: progress.value, next: next.value, commentId: autoCommentId.value, attachments: images.value.map(image => image.attachmentId).filter(Boolean), lastSaved: lastSaved.value})) } catch { /* Saving to the server remains available. */ }
})
function html(value: string) {
	return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\r?\n/g, '<br>')
}
async function save(automatic = false) {
	if (restoring.value || saving.value || (!progress.value.trim() && images.value.length === 0 && !autoCommentId.value) || !/^\d{4}-\d{2}-\d{2}$/.test(date.value)) return
	saving.value = true; message.value = ''
	const taskId = props.taskId
	try {
		let comment = `<h3>每日进展 · ${date.value}</h3><p>${html(progress.value.trim())}</p>`
			+ (next.value.trim() ? `<p><strong>遗留问题 / 下一步</strong></p><p>${html(next.value.trim())}</p>` : '')
		for (const picture of images.value) {
			if (!picture.attachmentId) {
				const result = await taskAttachmentsUpload({path: {task: taskId}, body: {files: [picture.file!]}})
				const attachment = result.data.success?.[0]
				if (!attachment?.id || result.data.errors?.length) throw new Error('Image upload failed')
				picture.attachmentId = attachment.id
			}
			comment += `<p><img src="/api/v1/tasks/${taskId}/attachments/${picture.attachmentId}" alt="进展图片"></p>`
		}
		if (snapshot() !== lastSaved.value) {
			if (autoCommentId.value) {
				await taskCommentsUpdate({path: {task: taskId, commentid: autoCommentId.value}, body: {comment}})
			} else {
				const result = await taskCommentsCreate({path: {task: taskId}, body: {comment}})
				autoCommentId.value = result.data.id
			}
			lastSaved.value = snapshot()
		}
		if (props.taskId !== taskId) return
		if (automatic) {
			message.value = `已自动保存 ${new Date().toLocaleTimeString()}，继续编辑会更新同一条记录。`
			emit('saved')
			return
		}
		autoCommentId.value = undefined
		while (images.value.length) removeImage(0)
		progress.value = ''; next.value = ''; message.value = '进展已保存，可在下方查看。'
		lastSaved.value = snapshot()
		emit('saved')
	} catch {
		if (props.taskId === taskId) message.value = '保存失败，内容已保留，请检查连接后重试。'
	} finally { saving.value = false }
}
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
