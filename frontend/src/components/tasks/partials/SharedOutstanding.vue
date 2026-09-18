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
		<p
			v-if="!loading && !visibleItems.length"
			class="outstanding-empty"
		>
			{{ completedItems.length ? '暂无未完成遗留事项' : '暂无遗留事项' }}
		</p>
		<ol
			v-if="visibleItems.length"
			class="outstanding-list"
		>
			<li
				v-for="entry in visibleItems"
				:key="entry.item.id"
				:value="entry.number"
				:class="{'is-completed': entry.item.done}"
			>
				<div class="outstanding-content">
					<input
						type="checkbox"
						:checked="entry.item.done"
						:disabled="blocked"
						:aria-label="`${entry.item.done ? '恢复' : '完成'}第 ${entry.number} 条遗留事项`"
						@change="toggleDone(entry.item, eventChecked($event))"
					>
					<ReadonlyRichText :html="entry.item.html" />
				</div>
				<div class="outstanding-actions">
					<button
						type="button"
						class="button is-small"
						:disabled="blocked"
						@click="selectItem(entry.item.id)"
					>
						{{ activeId === entry.item.id ? '正在编辑' : '编辑' }}
					</button>
					<button
						type="button"
						class="button is-small"
						:aria-label="`移除第 ${entry.number} 条遗留事项`"
						:disabled="blocked"
						@click="remove(entry.item.id)"
					>
						移除
					</button>
				</div>
			</li>
		</ol>
		<button
			v-if="completedItems.length"
			type="button"
			class="completed-toggle"
			:aria-expanded="showCompleted"
			@click="showCompleted = !showCompleted"
		>
			{{ showCompleted ? '隐藏已完成的遗留事项' : `显示已完成的遗留事项（${completedItems.length}）` }}
		</button>
		<ol
			v-if="showCompleted && completedItems.length"
			class="outstanding-list completed-list"
		>
			<li
				v-for="entry in completedItems"
				:key="entry.item.id"
				:value="entry.number"
				class="is-completed"
			>
				<div class="outstanding-content">
					<input
						type="checkbox"
						checked
						:disabled="blocked"
						:aria-label="`恢复第 ${entry.number} 条遗留事项`"
						@change="toggleDone(entry.item, false)"
					>
					<ReadonlyRichText :html="entry.item.html" />
				</div>
				<div class="outstanding-actions">
					<button
						type="button"
						class="button is-small"
						:disabled="blocked"
						@click="selectItem(entry.item.id)"
					>
						编辑
					</button>
					<button
						type="button"
						class="button is-small"
						:disabled="blocked"
						:aria-label="`移除第 ${entry.number} 条遗留事项`"
						@click="remove(entry.item.id)"
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
				<label :for="`outstanding-text-${taskId}`">{{ activeId ? activeIndex >= 0 ? `编辑第 ${activeIndex + 1} 条遗留事项` : '原遗留事项已移除或移动' : '新增遗留事项' }}</label>
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
				:placeholder="activeId ? '修改遗留事项内容，也可以添加或移除图片' : '输入一条遗留事项，也可以只添加图片'"
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
					{{ busy ? '正在保存…' : activeId ? '保存修改' : '添加遗留事项' }}
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
import {useTasktraceUndoGuard, undoInProgress} from '@/helpers/tasktraceUndo'
import {taskAttachmentsUpload} from '@/client/generated'
import {sharedOutstanding, readTaskHistory, changeOutstanding, type OutstandingItem} from '@/helpers/sharedOutstanding'
import ReadonlyRichText from './ReadonlyRichText.vue'

type ImageDraft = {file?: File, preview: string, attachmentId?: number}
type Draft = {text: string, images: ImageDraft[], itemId: string, original: string}
const props = defineProps<{taskId: number, disabled?: boolean}>()
const emit = defineEmits<{saved: [], busy: [value: boolean]}>()
const items = ref<OutstandingItem[]>([])
const drafts = reactive(new Map<string, Draft>())
const activeId = ref('')
const textInput = ref<HTMLTextAreaElement>()
const fileInput = ref<HTMLInputElement>()
const busy = ref(false)
watch(busy, value => emit('busy', value), {flush: 'sync'})
const loading = ref(false)
const error = ref('')
const message = ref('')
const showCompleted = ref(false)
const pendingCompletionIds = ref(new Set<string>())
const completionTimers = new Map<string, ReturnType<typeof setTimeout>>()
const blocked = computed(() => props.disabled || busy.value || loading.value || undoInProgress.value)
const activeIndex = computed(() => items.value.findIndex(item => item.id === activeId.value))
const numberedItems = computed(() => items.value.map((item, index) => ({item, number: index + 1})))
const visibleItems = computed(() => numberedItems.value.filter(entry => !entry.item.done || pendingCompletionIds.value.has(entry.item.id)))
const completedItems = computed(() => numberedItems.value.filter(entry => entry.item.done && !pendingCompletionIds.value.has(entry.item.id)))
const draft = computed(() => {
	const key = `${props.taskId}:${activeId.value}`
	if (!drafts.has(key)) {
		const fresh = {text: '', images: [], itemId: crypto.randomUUID(), original: ''}
		fresh.original = draftSignature(fresh)
		drafts.set(key, fresh)
	}
	return drafts.get(key)!
})
const canSave = computed(() => draft.value.images.length > 0 || !!draft.value.text.trim())
let loadVersion = 0
let mounted = true
useTasktraceUndoGuard(() => busy.value || [...drafts.values()].some(value => draftSignature(value) !== value.original), '请先保存或清空遗留事项的输入和待保存图片。')

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
	if (id) {
		const item = items.value.find(candidate => candidate.id === id)
		const key = `${props.taskId}:${id}`
		if (item && !drafts.has(key)) drafts.set(key, draftFromItem(item))
	}
	activeId.value = id
	error.value = ''
	message.value = ''
	await nextTick()
	textInput.value?.focus()
}

function draftFromItem(item: OutstandingItem): Draft {
	const doc = new DOMParser().parseFromString(item.html || '', 'text/html')
	const images = Array.from(doc.querySelectorAll('img')).map(image => {
		const preview = image.getAttribute('src') || ''
		const match = preview.match(/\/attachments\/(\d+)(?:$|[?#])/)
		return {preview, attachmentId: match ? Number(match[1]) : undefined}
	}).filter(image => !!image.preview)
	doc.querySelectorAll('img').forEach(image => image.remove())
	doc.querySelectorAll('br').forEach(line => line.replaceWith('\n'))
	const blocks = Array.from(doc.body.querySelectorAll('p, div, li')).map(block => block.textContent?.trim() || '').filter(Boolean)
	const result = {text: blocks.length ? blocks.join('\n') : doc.body.textContent?.trim() || '', images, itemId: item.id, original: ''}
	result.original = draftSignature(result)
	return result
}

function draftSignature(value: Pick<Draft, 'text' | 'images'>) {
	return JSON.stringify([value.text, value.images.map(image => [image.preview, image.attachmentId])])
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
		recovered.original = draftSignature({text: '', images: []})
		drafts.set(newKey, recovered)
	}
	drafts.delete(key)
	void selectItem('')
}

function eventChecked(event: Event) {
	return (event.target as HTMLInputElement).checked
}

function addImages(files: File[]) {
	if (blocked.value) return
	const images = files.filter(file => file.type.startsWith('image/'))
	for (const file of images) draft.value.images.push({file, preview: URL.createObjectURL(file)})
	message.value = images.length ? `已添加 ${images.length} 张图片，点击“${activeId.value ? '保存修改' : '添加遗留事项'}”保存。` : '请选择图片文件。'
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
	if (picture?.file) URL.revokeObjectURL(picture.preview)
}

function clearDraft(key: string) {
	const saved = drafts.get(key)
	saved?.images.filter(picture => picture.file).forEach(picture => URL.revokeObjectURL(picture.preview))
	drafts.delete(key)
}

function clearCompletionTimer(id: string) {
	const timer = completionTimers.get(id)
	if (timer) clearTimeout(timer)
	completionTimers.delete(id)
	if (pendingCompletionIds.value.has(id)) {
		const pending = new Set(pendingCompletionIds.value)
		pending.delete(id)
		pendingCompletionIds.value = pending
	}
}

function delayCompletedItem(id: string) {
	clearCompletionTimer(id)
	pendingCompletionIds.value = new Set([...pendingCompletionIds.value, id])
	completionTimers.set(id, setTimeout(() => {
		completionTimers.delete(id)
		const pending = new Set(pendingCompletionIds.value)
		pending.delete(id)
		pendingCompletionIds.value = pending
	}, 5000))
}

async function toggleDone(item: OutstandingItem, done: boolean) {
	if (blocked.value || item.done === done) return
	const taskId = props.taskId
	const previous = items.value
	const completedAt = done ? new Date().toISOString() : undefined
	items.value = items.value.map(candidate => candidate.id === item.id ? {...candidate, done, completedAt} : candidate)
	if (done) delayCompletedItem(item.id)
	else clearCompletionTimer(item.id)
	busy.value = true
	error.value = ''
	message.value = ''
	try {
		const result = await changeOutstanding(taskId, existing => {
			if (!existing.some(candidate => candidate.id === item.id)) throw new Error('Outstanding item no longer exists')
			return existing.map(candidate => candidate.id === item.id ? {...candidate, done, completedAt} : candidate)
		})
		if (taskId !== props.taskId || !mounted) return
		items.value = result
		message.value = done ? '已标记完成，5 秒后移入已完成遗留事项。' : '已恢复为未完成遗留事项。'
		emit('saved')
	} catch {
		if (taskId !== props.taskId || !mounted) return
		items.value = previous
		clearCompletionTimer(item.id)
		error.value = '完成状态保存失败，请重试。'
	} finally {
		busy.value = false
	}
}

function textHtml(value: string) {
	const paragraph = document.createElement('p')
	paragraph.textContent = value.trim()
	return value.trim() ? paragraph.outerHTML.replace(/\r?\n/g, '<br>') : ''
}

function imageHtml(source: string) {
	const paragraph = document.createElement('p')
	const image = document.createElement('img')
	image.setAttribute('src', source)
	image.alt = '遗留事项图片'
	paragraph.append(image)
	return paragraph.outerHTML
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
			if (!picture.file) continue
			const result = await taskAttachmentsUpload({path: {task: taskId}, body: {files: [picture.file]}})
			const attachmentId = result.data.success?.[0]?.id
			if (attachmentId) picture.attachmentId = attachmentId
			if (!attachmentId || result.data.errors?.length) throw new Error('Image upload failed')
		}
		const result = await changeOutstanding(taskId, existing => {
			const current = targetId ? existing.find(item => item.id === targetId) : undefined
			if (targetId && !current) throw new Error('Outstanding item no longer exists')
			const images = savedDraft.images.map(picture => picture.attachmentId ? `/api/v1/tasks/${taskId}/attachments/${picture.attachmentId}` : picture.preview)
			const html = `${textHtml(savedDraft.text)}${images.map(imageHtml).join('')}`
			if (current) return existing.map(item => item.id === targetId ? {...item, html} : item)
			const added = {id: savedDraft.itemId, html, done: false, priority: 9}
			return existing.some(item => item.id === added.id) ? existing.map(item => item.id === added.id ? added : item) : [...existing, added]
		})
		clearDraft(key)
		if (taskId !== props.taskId || !mounted) return
		items.value = result
		activeId.value = ''
		message.value = targetId ? '遗留事项修改已保存。' : '遗留事项已保存。'
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
		clearCompletionTimer(id)
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
	for (const id of completionTimers.keys()) clearCompletionTimer(id)
	activeId.value = ''
	items.value = []
	showCompleted.value = false
	message.value = ''
	void load()
}, {immediate: true})
onBeforeUnmount(() => {
	mounted = false
	emit('busy', false)
	++loadVersion
	for (const id of completionTimers.keys()) clearCompletionTimer(id)
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

.outstanding-content {
	display: grid;
	grid-template-columns: auto minmax(0, 1fr);
	align-items: start;
	gap: .55rem;

	input { margin-block-start: .3rem; }
}

.is-completed .readonly-rich-text {
	color: var(--grey-600);
	text-decoration: line-through;
}

.completed-toggle {
	justify-self: start;
	border: 0;
	padding: 0;
	background: transparent;
	color: var(--grey-600);
	font-size: .875rem;
	cursor: pointer;
	text-decoration: underline;
	text-underline-offset: .15em;

	&:hover { color: var(--primary); }

	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: 2px;
	}
}

.completed-list { margin-block-start: -.2rem; }
.outstanding-empty {
	margin: 0;
	color: var(--grey-600);
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
