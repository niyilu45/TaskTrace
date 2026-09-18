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
			:disabled="saving || referenceLoading"
			@click="switchDate(date, true)"
		>
			重新读取当天进展
		</button>
		<label :for="`progress-date-${taskId}`">记录日期</label>
		<ProgressDatePicker
			:id="`progress-date-${taskId}`"
			:model-value="date"
			:marked-dates="progressDates"
			:disabled="saving || referenceLoading"
			@update:modelValue="switchDate"
		/>
		<label :for="`progress-text-${taskId}`">今日进展</label>
		<textarea
			:id="`progress-text-${taskId}`"
			ref="progressTextarea"
			v-model="progress"
			class="textarea daily-progress__textarea"
			rows="3"
			placeholder="今天完成了什么？"
			:disabled="saving || restoring"
		/>
		<div class="reference-picker">
			<label>引用历史进展</label>
			<button
				type="button"
				class="button"
				:disabled="saving || restoring || referenceLoading || referenceDates.length === 0"
				@click="openReferencePicker"
			>
				{{ referenceLoading ? '正在读取…' : '引用历史进展' }}
			</button>
			<p class="reference-hint">
				在表格中勾选一个或多个更早日期。引用保留当时内容，原记录不变。
			</p>
		</div>
		<section
			v-if="references.length"
			aria-label="已引用的历史进展"
			class="progress-references"
		>
			<button
				type="button"
				class="reference-toggle"
				:aria-expanded="referencesExpanded"
				@click="referencesExpanded = !referencesExpanded"
			>
				{{ referencesExpanded ? '收起引用的历史进展' : `展开引用的历史进展（${references.length}）` }}
			</button>
			<template v-if="referencesExpanded">
				<article
					v-for="reference in references"
					:key="reference.id"
					class="progress-reference"
					:data-reference-date="reference.date"
				>
					<div class="reference-heading">
						<strong>引用 {{ reference.date }} 的进展</strong>
						<a
							:href="`/tasks/${reference.taskId}#comment-${Math.max(...reference.commentIds)}`"
							target="_blank"
							rel="noopener noreferrer"
						>查看原记录</a>
						<button
							type="button"
							class="button is-small"
							:aria-label="`移除 ${reference.date} 的引用`"
							:disabled="saving || restoring || referenceLoading"
							@click="references = references.filter(item => item.id !== reference.id)"
						>
							移除引用
						</button>
					</div>
					<ReadonlyRichText :html="reference.html" />
				</article>
			</template>
		</section>
		<Modal
			:enabled="showReferencePicker"
			aria-label="引用历史进展"
			wide
			@close="cancelReferencePicker"
		>
			<section
				class="reference-dialog"
				aria-labelledby="reference-dialog-title"
			>
				<h2 id="reference-dialog-title">
					引用历史进展
				</h2>
				<p>勾选需要引用的历史进展，可同时选择多个日期。</p>
				<div class="reference-table-wrap">
					<table class="reference-table">
						<thead>
							<tr>
								<th scope="col">
									选择
								</th><th scope="col">
									日期
								</th><th scope="col">
									历史进展信息
								</th>
							</tr>
						</thead>
						<tbody>
							<tr
								v-for="candidate in referenceCandidates"
								:key="candidate.date"
							>
								<td>
									<input
										v-model="selectedReferenceDates"
										type="checkbox"
										:value="candidate.date"
										:aria-label="`引用 ${candidate.date} 的进展`"
									>
								</td>
								<th scope="row">
									{{ candidate.date }}
								</th>
								<td><ReadonlyRichText :html="candidate.html" /></td>
							</tr>
						</tbody>
					</table>
				</div>
				<div class="reference-dialog__actions">
					<button
						type="button"
						class="button"
						@click="cancelReferencePicker"
					>
						取消
					</button>
					<button
						type="button"
						class="button is-primary"
						:disabled="selectedReferenceDates.length === 0 || referenceLoading"
						@click="confirmReferences"
					>
						{{ referenceLoading ? '正在添加…' : '确定' }}
					</button>
				</div>
			</section>
		</Modal>
		<SharedOutstanding
			:task-id="taskId"
			:disabled="saving || referenceLoading"
			@saved="emit('saved')"
			@busy="sharedBusy = $event"
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
				:disabled="saving || referenceLoading || restoring || sharedBusy || (!progress.trim() && images.length === 0 && !references.length && !autoCommentId)"
			>
				{{ saving ? '正在保存…' : '保存进展' }}
			</button>
			<span role="status">{{ message || 'Ctrl + Enter 快速保存，历史记录保留在下方。' }}</span>
		</div>
	</form>
</template>

<script setup lang="ts">
import {useTasktraceUndoGuard, undoInProgress, undoGroupHeaders} from '@/helpers/tasktraceUndo'
import {ref, reactive, computed, watch, onBeforeUnmount} from 'vue'
import {useAutoHeightTextarea} from '@/composables/useAutoHeightTextarea'
import {taskCommentsCreate, taskCommentsUpdate, taskAttachmentsUpload} from '@/client/generated'
import AutoSaveSettings from './AutoSaveSettings.vue'
import ProgressDatePicker from './ProgressDatePicker.vue'
import SharedOutstanding from './SharedOutstanding.vue'
import ReadonlyRichText from './ReadonlyRichText.vue'
import {readTaskHistory, sharedOutstanding, changeOutstanding} from '@/helpers/sharedOutstanding'
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'
import {mergedDay, sortProgressNotes} from '@/helpers/progressNotes'
import {createProgressReference, normalizeProgressReferences, serializeProgressReferences, type ProgressReference} from '@/helpers/progressReferences'
import {useAutoSave} from '@/helpers/autoSave'
import {useAuthStore} from '@/stores/auth'
const props = defineProps<{taskId: number}>()
const emit = defineEmits<{saved: []}>()
const authStore = useAuthStore()
const date = ref('')
const progress = ref('')
const {textarea: progressTextarea} = useAutoHeightTextarea(progress)
const images = ref<{file?: File, preview: string, attachmentId?: number}[]>([])
const existingImages = ref('')
const originalHtml = ref('')
const originalText = ref('')
const mergedIds = ref<number[]>([])
const autoCommentId = ref<number>()
const saving = ref(false)
const sharedBusy = ref(false)
const references = ref<ProgressReference[]>([])
const showReferencePicker = ref(false)
const selectedReferenceDates = ref<string[]>([])
const referencesExpanded = ref(false)
const referenceLoading = ref(false)
const referenceHistory = ref<Awaited<ReturnType<typeof readTaskHistory>>>([])
const progressDates = computed(() => {
	const dates = new Set(sortProgressNotes(referenceHistory.value).filter(note => note.daily).map(note => note.date))
	if (autoCommentId.value && date.value) dates.add(date.value)
	return [...dates]
})
const referenceDates = computed(() => [...new Set(sortProgressNotes(referenceHistory.value)
	.filter(note => note.daily && note.date < date.value && !references.value.some(reference => reference.date === note.date))
	.map(note => note.date))].sort().reverse())
const referenceCandidates = computed(() => referenceDates.value.map(day => ({date: day, html: mergedDay(referenceHistory.value, day).html})))
const restoring = ref(true)
const message = ref('')
const lastSaved = ref('')
let version = 0
const snapshot = () => JSON.stringify([date.value, progress.value, images.value.map(image => image.attachmentId || image.preview), references.value])
const drafts = reactive(new Map<string, {text: string, images: typeof images.value, references: ProgressReference[]}>())
useTasktraceUndoGuard(() => saving.value || referenceLoading.value || sharedBusy.value || drafts.size > 0 || (!restoring.value && snapshot() !== lastSaved.value), '请先保存每日进展及其他日期的草稿。')
function stash() {
	if (undoInProgress.value || restoring.value || !date.value) return
	if (snapshot() === lastSaved.value) {
		drafts.delete(`${props.taskId}:${date.value}`)
		try {
			localStorage.removeItem(`tasktrace-day-draft-${props.taskId}-${date.value}`)
			const legacy = JSON.parse(localStorage.getItem(`tasktrace-progress-draft-${props.taskId}`) || 'null')
			if (legacy?.date === date.value) localStorage.removeItem(`tasktrace-progress-draft-${props.taskId}`)
		} catch { /* Optional draft cleanup. */ }
		return
	}
	drafts.set(`${props.taskId}:${date.value}`, {text: progress.value, images: [...images.value], references: normalizeProgressReferences(references.value)})
	try { localStorage.setItem(`tasktrace-day-draft-${props.taskId}-${date.value}`, JSON.stringify({progress: progress.value, references: references.value, attachments: images.value.map(image => image.attachmentId).filter(Boolean)})) } catch { /* Server save remains available. */ }
}
async function switchDate(value: string, initial = false) {
	if (saving.value || referenceLoading.value) { message.value = '正在保存或读取引用，请完成后再切换日期。'; return false }
	if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
	if (!initial) stash()
	const request = ++version
	const taskId = props.taskId
	restoring.value = true; date.value = value; message.value = ''
	let loaded = false
	try {
		const history = await readTaskHistory(taskId)
		if (request !== version || taskId !== props.taskId) return false
		referenceHistory.value = history; showReferencePicker.value = false; selectedReferenceDates.value = []
		const selected = mergedDay(history, value, authStore.info?.username)
		date.value = value; autoCommentId.value = selected.id; mergedIds.value = selected.mergedIds
		originalHtml.value = selected.html; originalText.value = selected.text; existingImages.value = selected.images
		progress.value = selected.text; images.value = []; references.value = normalizeProgressReferences(selected.references)
		let draft = drafts.get(`${taskId}:${value}`)
		if (!draft) {
			try {
				let saved = JSON.parse(localStorage.getItem(`tasktrace-day-draft-${taskId}-${value}`) || 'null')
				const legacy = JSON.parse(localStorage.getItem(`tasktrace-progress-draft-${taskId}`) || 'null')
				if (!saved && legacy?.date === value) saved = legacy
				if (saved && typeof saved.progress === 'string') {
					const pictures: typeof images.value = []
					for (const id of saved.attachments || []) if (Number.isInteger(id) && id > 0) pictures.push({attachmentId: id, preview: await fetchAttachmentBlobUrl({taskId, id})})
					draft = {text: saved.progress, images: pictures, references: normalizeProgressReferences(saved.references)}
				}
			} catch { message.value = '草稿恢复失败，已保留服务器内容。' }
		}
		if (request !== version || taskId !== props.taskId) return false
		lastSaved.value = snapshot()
		if (draft) { progress.value = draft.text; images.value = draft.images; references.value = normalizeProgressReferences(draft.references) }
		loaded = true
		message.value = selected.id ? '已载入当天进展；同日记录合并编辑，保存会更新当天内容。' : '此日期尚无进展。'
	} catch { message.value = '历史读取失败，已暂停保存，请重新选择日期重试。'; return false }
	finally { if (request === version) restoring.value = !loaded }
	return loaded
}
watch(() => props.taskId, async () => {
	const now = new Date()
	const today = `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,'0')}-${String(now.getDate()).padStart(2,'0')}`
	await switchDate(today, true)
}, {immediate: true})
watch([progress, images, references], stash, {deep: true})
async function openReferencePicker() {
	if (saving.value || restoring.value || referenceLoading.value) return
	const taskId = props.taskId
	const request = version
	referenceLoading.value = true
	try {
		const history = await readTaskHistory(taskId)
		if (request !== version || taskId !== props.taskId) return
		referenceHistory.value = history
		selectedReferenceDates.value = []
		showReferencePicker.value = true
	} catch { message.value = '历史进展读取失败，请重试。' }
	finally { referenceLoading.value = false }
}
function cancelReferencePicker() {
	showReferencePicker.value = false
	selectedReferenceDates.value = []
}
async function confirmReferences() {
	if (saving.value || restoring.value || referenceLoading.value || selectedReferenceDates.value.length === 0) return
	const taskId = props.taskId
	const request = version
	const selected = [...selectedReferenceDates.value]
	referenceLoading.value = true
	try {
		const history = await readTaskHistory(taskId)
		if (request !== version || taskId !== props.taskId) return
		referenceHistory.value = history
		let added = 0
		for (const sourceDate of selected) {
			if (references.value.some(item => item.date === sourceDate)) continue
			const reference = createProgressReference(taskId, sourceDate, date.value, mergedDay(history, sourceDate))
			if (reference) { references.value.push(reference); added++ }
		}
		showReferencePicker.value = false
		selectedReferenceDates.value = []
		if (added) {
			referencesExpanded.value = true
			message.value = `已添加 ${added} 条引用，请在今日进展中填写更正说明。`
		} else {
			message.value = '所选日期已无可引用的进展，请重新选择。'
		}
	} catch { message.value = '引用读取失败，现有内容已保留，请重试。' }
	finally { referenceLoading.value = false }
}
defineExpose({switchDate})
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
	if (undoInProgress.value || restoring.value || saving.value || referenceLoading.value || sharedBusy.value || (!progress.value.trim() && images.value.length === 0 && !references.value.length && !autoCommentId.value)) return
	saving.value = true
	const taskId = props.taskId
	const undoHeaders = undoGroupHeaders()
	try {
		const latestHistory = await readTaskHistory(taskId)
		if (!sharedOutstanding(latestHistory).id) await changeOutstanding(taskId, items => items, undoHeaders)
		let body = progress.value === originalText.value ? originalHtml.value : `<p>${html(progress.value.trim())}</p>${existingImages.value}`
		for (const picture of images.value) {
			if (!picture.attachmentId) {
				const result = await taskAttachmentsUpload({path: {task: taskId}, body: {files: [picture.file!]}})
				if (!result.data.success?.[0]?.id || result.data.errors?.length) throw new Error('Upload failed')
				picture.attachmentId = result.data.success[0].id
			}
			body += `<p><img src="/api/v1/tasks/${taskId}/attachments/${picture.attachmentId}" alt="进展图片"></p>`
		}
		const comment = `<h3 data-tasktrace-merged="${mergedIds.value.join(',')}">每日进展 · ${date.value}</h3>${body}${serializeProgressReferences(references.value)}`
		if (snapshot() !== lastSaved.value || mergedIds.value.length) {
			if (autoCommentId.value) await taskCommentsUpdate({path: {task: taskId, commentid: autoCommentId.value}, body: {comment}, headers: undoHeaders})
			else autoCommentId.value = (await taskCommentsCreate({path: {task: taskId}, body: {comment}, headers: undoHeaders})).data.id
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
useAutoSave(async () => { if (!restoring.value && !sharedBusy.value && snapshot() !== lastSaved.value) await save(true) })
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
.daily-progress__textarea {
	resize: none;
}
.reference-heading {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: .65rem;
	font-size: .875rem;
}
.reference-picker label {
	display: block;
	margin-block-end: .5rem;
}
.reference-hint {
	font-size: .875rem;
	color: var(--grey-600);
}
.progress-references {
	display: grid;
	gap: .75rem;
	min-inline-size: 0;
}
.progress-reference {
	border-inline-start: 3px solid var(--grey-300);
	padding: .5rem .75rem;
	background: var(--grey-50);
}
.reference-toggle {
	justify-self: start;
	border: 0;
	padding: 0;
	background: transparent;
	color: var(--primary);
	font: inherit;
	cursor: pointer;
	text-decoration: underline;
	text-underline-offset: .15em;
}
.reference-table-wrap {
	max-block-size: min(55vh, 30rem);
	overflow: auto;
	border: 1px solid var(--grey-200);
	border-radius: $radius;
}
.reference-table {
	inline-size: 100%;
	border-collapse: collapse;
	th, td {
		padding: .65rem;
		border-block-end: 1px solid var(--grey-200);
		text-align: start;
		vertical-align: top;
	}
	thead th {
		position: sticky;
		inset-block-start: 0;
		z-index: 1;
		background: var(--grey-50);
	}
	tbody tr:last-child > * { border-block-end: 0; }
	tbody th { white-space: nowrap; }
	input {
		inline-size: 1rem;
		block-size: 1rem;
	}
}
.reference-dialog {
	inline-size: min(60rem, calc(100vw - 3rem));
	max-block-size: calc(100vh - 4rem);
	padding: 1.25rem;
	border-radius: $radius;
	background: var(--white);
	overflow: auto;

	h2 { margin-block: 0 .5rem; }
}
.reference-dialog__actions {
	display: flex;
	justify-content: flex-end;
	gap: .65rem;
	margin-block-start: 1rem;
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
