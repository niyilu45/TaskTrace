<template>
	<section
		class="shared-outstanding"
		aria-label="共享遗留事项"
		@paste="pasteImages"
		@keydown.ctrl.enter.stop.prevent="save"
	>
		<h4>遗留事项（所有日期共享）</h4>
		<p
			v-if="loading"
			role="status"
		>
			正在读取…
		</p>
		<ol class="outstanding-list">
			<li
				v-for="(item, index) in items"
				:key="item.id"
			>
				<ReadonlyRichText :html="item.html" />
				<div class="outstanding-actions">
					<button
						type="button"
						class="button is-small"
						:disabled="blocked"
						@click="selectItem(item.id)"
					>
						{{ activeId === item.id ? '正在添加图片' : '添加图片' }}
					</button>
					<button
						type="button"
						class="button is-small"
						:aria-label="`移除第 ${index + 1} 条遗留事项`"
						:disabled="blocked"
						@click="remove(item.id)"
					>
						移除
					</button>
				</div>
			</li>
		</ol>
		<div
			class="outstanding-composer"
			:aria-busy="busy"
		>
			<div class="outstanding-actions">
				<label :for="`outstanding-text-${taskId}`">{{ activeId ? activeIndex >= 0 ? `为第 ${activeIndex + 1} 条遗留事项添加图片` : '原遗留事项已移除或移动' : '新增遗留事项' }}</label>
				<button
					v-if="activeId && activeIndex < 0 && !loading"
					type="button"
					class="button is-small"
					:disabled="blocked"
					@click="recoverDraft"
				>
					另存为新遗留事项
				</button>
				<button
					v-if="activeId"
					type="button"
					class="button is-small"
					:disabled="blocked"
					@click="selectItem('')"
				>
					返回新增事项
				</button>
			</div>
			<textarea
				:id="`outstanding-text-${taskId}`"
				ref="textInput"
				v-model="draft.text"
				class="textarea"
				rows="2"
				:placeholder="activeId ? '图片说明（可选）' : '输入一条遗留事项，也可以只添加图片'"
				:aria-describedby="`outstanding-hint-${taskId}`"
				:disabled="blocked"
			/>
			<p
				:id="`outstanding-hint-${taskId}`"
				class="outstanding-hint"
			>
				在此处按 Ctrl+V 粘贴图片，或选择图片文件；保存后所有日期共享。
			</p>
			<div
				v-if="draft.images.length"
				class="outstanding-images"
			>
				<figure
					v-for="(picture, index) in draft.images"
					:key="picture.preview"
				>
					<img
						:src="picture.preview"
						:alt="`待保存的遗留事项图片 ${index + 1}`"
					>
					<button
						type="button"
						class="button is-small"
						:disabled="blocked"
						@click="removeImage(index)"
					>
						移除图片 {{ index + 1 }}
					</button>
				</figure>
			</div>
			<div class="outstanding-actions">
				<input
					ref="fileInput"
					class="outstanding-file-input"
					type="file"
					accept="image/*"
					multiple
					:disabled="blocked"
					tabindex="-1"
					aria-label="选择遗留事项图片"
					@change="chooseImages"
				>
				<button
					type="button"
					class="button"
					:disabled="blocked"
					@click="fileInput?.click()"
				>
					选择图片
				</button>
				<button
					type="button"
					class="button is-primary"
					:disabled="blocked || !canSave"
					@click="save"
				>
					{{ busy ? '正在保存…' : activeId ? '保存图片' : '添加遗留事项' }}
				</button>
			</div>
		</div>
		<p
			v-if="message"
			aria-live="polite"
		>
			{{ message }}
		</p>
		<p
			v-if="error"
			role="alert"
		>
			{{ error }}
			<button
				type="button"
				class="button is-small"
				:disabled="blocked"
				@click="load"
			>
				重新读取
			</button>
		</p>
	</section>
</template>

<script setup lang="ts">
import {computed, nextTick, onBeforeUnmount, reactive, ref, watch} from 'vue'
import {taskAttachmentsUpload} from '@/client/generated'
import {sharedOutstanding, readTaskHistory, changeOutstanding, type OutstandingItem} from '@/helpers/sharedOutstanding'
import ReadonlyRichText from './ReadonlyRichText.vue'

type ImageDraft = {file: File, preview: string, attachmentId?: number}
type Draft = {text: string, images: ImageDraft[], itemId: string}
const props = defineProps<{taskId: number, disabled?: boolean}>()
const emit = defineEmits<{saved: []}>()
const items = ref<OutstandingItem[]>([])
const drafts = reactive(new Map<string, Draft>())
const activeId = ref('')
const textInput = ref<HTMLTextAreaElement>()
const fileInput = ref<HTMLInputElement>()
const busy = ref(false)
const loading = ref(false)
const error = ref('')
const message = ref('')
const blocked = computed(() => props.disabled || busy.value || loading.value)
const activeIndex = computed(() => items.value.findIndex(item => item.id === activeId.value))
const draft = computed(() => {
	const key = `${props.taskId}:${activeId.value}`
	if (!drafts.has(key)) drafts.set(key, {text: '', images: [], itemId: crypto.randomUUID()})
	return drafts.get(key)!
})
const canSave = computed(() => draft.value.images.length > 0 || (!activeId.value && !!draft.value.text.trim()))
let loadVersion = 0
let mounted = true

async function load() {
	const taskId = props.taskId
	const version = ++loadVersion
	loading.value = true
	error.value = ''
	try {
		const history = await readTaskHistory(taskId)
		if (version === loadVersion && mounted) items.value = sharedOutstanding(history).items
	} catch {
		if (version === loadVersion && mounted) error.value = '读取失败，请重试。输入和待保存图片已保留。'
	} finally {
		if (version === loadVersion && mounted) loading.value = false
	}
}

async function selectItem(id: string) {
	activeId.value = id
	error.value = ''
	message.value = ''
	await nextTick()
	textInput.value?.focus()
}

function recoverDraft() {
	if (blocked.value || !activeId.value) return
	const key = `${props.taskId}:${activeId.value}`
	const recovered = draft.value
	const newKey = `${props.taskId}:`
	const pending = drafts.get(newKey)
	if (pending) {
		pending.text = [pending.text, recovered.text].filter(Boolean).join('\n')
		pending.images.push(...recovered.images)
	} else {
		drafts.set(newKey, recovered)
	}
	drafts.delete(key)
	void selectItem('')
}

function addImages(files: File[]) {
	if (blocked.value) return
	const images = files.filter(file => file.type.startsWith('image/'))
	for (const file of images) draft.value.images.push({file, preview: URL.createObjectURL(file)})
	message.value = images.length ? `已添加 ${images.length} 张图片，点击“${activeId.value ? '保存图片' : '添加遗留事项'}”保存。` : '请选择图片文件。'
}

function pasteImages(event: ClipboardEvent) {
	const files = Array.from(event.clipboardData?.items || [])
		.filter(item => item.kind === 'file' && item.type.startsWith('image/'))
		.map(item => item.getAsFile()).filter((file): file is File => !!file)
	if (!files.length) return
	event.preventDefault()
	event.stopPropagation()
	addImages(files)
}

function chooseImages(event: Event) {
	const input = event.target as HTMLInputElement
	addImages(Array.from(input.files || []))
	input.value = ''
}

function removeImage(index: number) {
	const [picture] = draft.value.images.splice(index, 1)
	if (picture) URL.revokeObjectURL(picture.preview)
}

function clearDraft(key: string) {
	const saved = drafts.get(key)
	saved?.images.forEach(picture => URL.revokeObjectURL(picture.preview))
	drafts.delete(key)
}

function textHtml(value: string) {
	const paragraph = document.createElement('p')
	paragraph.textContent = value.trim()
	return value.trim() ? paragraph.outerHTML.replace(/\r?\n/g, '<br>') : ''
}

async function save() {
	if (blocked.value || !canSave.value) return
	const taskId = props.taskId
	const targetId = activeId.value
	const key = `${taskId}:${targetId}`
	const savedDraft = draft.value
	busy.value = true
	error.value = ''
	message.value = ''
	try {
		for (const picture of savedDraft.images) {
			if (picture.attachmentId) continue
			const result = await taskAttachmentsUpload({path: {task: taskId}, body: {files: [picture.file]}})
			const attachmentId = result.data.success?.[0]?.id
			if (attachmentId) picture.attachmentId = attachmentId
			if (!attachmentId || result.data.errors?.length) throw new Error('Image upload failed')
		}
		const result = await changeOutstanding(taskId, existing => {
			const current = targetId ? existing.find(item => item.id === targetId) : undefined
			if (targetId && !current) throw new Error('Outstanding item no longer exists')
			const presentImages = new Set(Array.from(new DOMParser().parseFromString(current?.html || '', 'text/html').querySelectorAll('img')).map(image => image.getAttribute('src')))
			const images = savedDraft.images.map(picture => `/api/v1/tasks/${taskId}/attachments/${picture.attachmentId}`).filter(source => !presentImages.has(source))
			const html = `${textHtml(savedDraft.text)}${images.map(source => `<p><img src="${source}" alt="遗留事项图片"></p>`).join('')}`
			if (current) return existing.map(item => item.id === targetId ? {...item, html: item.html + (images.length ? html : '')} : item)
			const added = {id: savedDraft.itemId, html}
			return existing.some(item => item.id === added.id) ? existing.map(item => item.id === added.id ? added : item) : [...existing, added]
		})
		clearDraft(key)
		if (taskId !== props.taskId || !mounted) return
		items.value = result
		activeId.value = ''
		message.value = targetId ? '图片已添加到遗留事项。' : '遗留事项已保存。'
		emit('saved')
	} catch (cause) {
		if (taskId !== props.taskId || !mounted) return
		error.value = cause instanceof Error && cause.message === 'Outstanding item no longer exists'
			? '这条遗留事项已被移除或移动。输入和图片已保留，请重新读取。'
			: '保存失败，输入和图片已保留，请重试。'
	} finally {
		busy.value = false
	}
}

async function remove(id: string) {
	if (blocked.value) return
	const taskId = props.taskId
	busy.value = true
	error.value = ''
	try {
		const result = await changeOutstanding(taskId, existing => existing.filter(item => item.id !== id))
		clearDraft(`${taskId}:${id}`)
		if (taskId !== props.taskId || !mounted) return
		items.value = result
		if (activeId.value === id) activeId.value = ''
		emit('saved')
	} catch {
		if (taskId === props.taskId && mounted) error.value = '移除失败，请重试。'
	} finally {
		busy.value = false
	}
}

watch(() => props.taskId, () => {
	activeId.value = ''
	items.value = []
	message.value = ''
	void load()
}, {immediate: true})
onBeforeUnmount(() => {
	mounted = false
	++loadVersion
	for (const key of drafts.keys()) clearDraft(key)
})
</script>

<style scoped lang="scss">
.shared-outstanding {
	display: grid;
	gap: .65rem;
	min-inline-size: 0;
}

.outstanding-list {
	margin: 0;
	padding-inline-start: 1.75rem;
	list-style: decimal;

	li { padding-block-end: .75rem; }
	li::marker { font-weight: 600; }

	.readonly-rich-text {
		display: inline-block;
		inline-size: 100%;
		vertical-align: top;
	}
}

.outstanding-composer {
	display: grid;
	gap: .65rem;

	label { font-weight: 600; }
}

.outstanding-actions {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: .5rem;
}

.outstanding-hint { font-size: .875rem; }
.outstanding-file-input { display: none; }

.outstanding-images {
	display: flex;
	flex-wrap: wrap;
	gap: .75rem;

	figure {
		display: grid;
		gap: .4rem;
		margin: 0;
	}

	img {
		inline-size: 9rem;
		max-inline-size: 100%;
		block-size: 6rem;
		object-fit: contain;
	}
}
</style>
