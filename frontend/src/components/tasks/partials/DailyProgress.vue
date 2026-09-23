<template>
	<section class="daily-progress">
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
		<section
			class="daily-progress-editors"
			aria-label="当天各成员进展"
		>
			<div class="daily-progress__heading-row">
				<div>
					<label :for="`progress-text-${taskId}`">我的进展</label>
					<p>我的编辑框始终保留。图片直接插入正文，选中后可按退格或 Delete 删除。</p>
				</div>
				<button
					class="button is-primary"
					type="button"
					:disabled="saving || referenceLoading || restoring || sharedBusy || !canSave"
					@click="save"
				>
					{{ saving ? '正在保存…' : '保存进展' }}
				</button>
			</div>
			<div
				:id="`progress-text-${taskId}`"
				class="daily-progress__editor"
				tabindex="-1"
			>
				<Editor
					v-model="progress"
					:always-editing="true"
					:allow-base64-images="true"
					:upload-callback="stageProgressImages"
					placeholder="今天完成了什么？可直接粘贴图片。"
					@save="save"
				/>
			</div>
			<p class="progress-image-count">
				正文内有 {{ imageCount }} 张图片。自动保存只缓存到 .cache；点击“保存进展”后才会正式提交。
			</p>

			<section
				v-if="otherAuthorsForDate.length"
				class="member-progress-list"
				aria-label="其他成员当天进展"
			>
				<h4>其他成员在当天提交的进展</h4>
				<DailyProgressMemberEditor
					v-for="author in otherAuthorsForDate"
					:key="`${date}:${author.toLowerCase()}`"
					:task-id="taskId"
					:date="date"
					:author="author"
					:editor="currentUsername"
					:history="referenceHistory"
					:data-progress-author-key="teamMemberKey(author)"
					@saved="memberSaved"
				/>
			</section>
		</section>

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

		<div class="daily-progress-actions">
			<span role="status">{{ message || 'Ctrl + Enter 快速保存，历史记录保留在下方。' }}</span>
		</div>
	</section>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, reactive, ref, watch} from 'vue'
import {taskCommentsCreate, taskCommentsUpdate} from '@/client/generated'
import Editor from '@/components/input/AsyncEditor'
import AutoSaveSettings from './AutoSaveSettings.vue'
import DailyProgressMemberEditor from './DailyProgressMemberEditor.vue'
import ProgressDatePicker from './ProgressDatePicker.vue'
import SharedOutstanding from './SharedOutstanding.vue'
import ReadonlyRichText from './ReadonlyRichText.vue'
import {readTaskHistory, sharedOutstanding, changeOutstanding} from '@/helpers/sharedOutstanding'
import {finalProgressNotes, mergedDay, sortProgressNotes} from '@/helpers/progressNotes'
import {createProgressReference, normalizeProgressReferences, serializeProgressReferences, type ProgressReference} from '@/helpers/progressReferences'
import {autoSaveSettings, useAutoSave} from '@/helpers/autoSave'
import {useAuthStore} from '@/stores/auth'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import {createTeamCommentId, serializeTeamCommentMarker} from '@/helpers/tasktraceTeam'
import {teamMemberKey} from '@/helpers/tasktraceTeamMembers'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {deleteTaskTraceDraft, readTaskTraceDraft, writeTaskTraceDraft} from '@/helpers/tasktraceDraftCache'
import {countProgressImages, persistProgressImages, stageProgressImages} from '@/helpers/progressEditorImages'
import {isEditorContentEmpty} from '@/helpers/editorContentEmpty'
import {useTasktraceUndoGuard, undoGroupHeaders, undoInProgress} from '@/helpers/tasktraceUndo'

const props = defineProps<{taskId: number}>()
const emit = defineEmits<{saved: []}>()
const authStore = useAuthStore()
const teamStore = useTasktraceTeamStore()
const date = ref('')
const progress = ref('')
const currentUsername = computed(() => authStore.info?.username || teamStore.status.username || '')
const binding = computed(() => teamStore.bindingForTask(props.taskId))
const collaborative = computed(() => !!binding.value)
const mergedIds = ref<number[]>([])
const mergedTeamIds = ref<string[]>([])
const autoCommentId = ref<number>()
const autoTeamId = ref('')
const saving = ref(false)
const sharedBusy = ref(false)
const references = ref<ProgressReference[]>([])
const showReferencePicker = ref(false)
const selectedReferenceDates = ref<string[]>([])
const referencesExpanded = ref(false)
const referenceLoading = ref(false)
const referenceHistory = ref<Awaited<ReturnType<typeof readTaskHistory>>>([])
const restoring = ref(true)
const message = ref('')
const lastSaved = ref('')
let version = 0
let historyRefreshVersion = 0

const imageCount = computed(() => countProgressImages(progress.value))
const canSave = computed(() => !!autoCommentId.value || references.value.length > 0 || imageCount.value > 0 || !isEditorContentEmpty(progress.value))
const progressDates = computed(() => {
	const dates = new Set(sortProgressNotes(referenceHistory.value).filter(note => note.daily).map(note => note.date))
	if (autoCommentId.value && date.value) dates.add(date.value)
	return [...dates]
})
const otherAuthorsForDate = computed(() => {
	const current = teamMemberKey(currentUsername.value)
	const result: string[] = []
	for (const note of finalProgressNotes(referenceHistory.value)) {
		if (!note.daily || note.date !== date.value || !note.author || teamMemberKey(note.author) === current) continue
		if (!result.some(author => teamMemberKey(author) === teamMemberKey(note.author))) result.push(note.author.trim())
	}
	return result
})
const referenceDates = computed(() => [...new Set(sortProgressNotes(referenceHistory.value)
	.filter(note => note.daily && note.date < date.value && !references.value.some(reference => reference.date === note.date))
	.map(note => note.date))].sort().reverse())
const referenceCandidates = computed(() => referenceDates.value.map(day => ({date: day, html: mergedDay(referenceHistory.value, day).html})))

type ProgressDraft = {html: string, references: ProgressReference[]}
type CachedProgressDraft = {progress?: string, references?: ProgressReference[]}
const drafts = reactive(new Map<string, ProgressDraft>())
const cachedSnapshots = new Map<string, string>()
const snapshot = () => JSON.stringify([date.value, progress.value, references.value])
const draftKey = (taskId = props.taskId, day = date.value) => `${taskId}:${day}`

useTasktraceUndoGuard(() => saving.value || referenceLoading.value || sharedBusy.value || drafts.size > 0 || (!restoring.value && snapshot() !== lastSaved.value), '请先保存每日进展及其他日期的草稿。')

function stash() {
	if (undoInProgress.value || restoring.value || !date.value) return
	const key = draftKey()
	if (snapshot() === lastSaved.value) {
		drafts.delete(key)
		return
	}
	drafts.set(key, {html: progress.value, references: normalizeProgressReferences(references.value)})
}

async function cacheChangedDrafts() {
	let changed = false
	const prefix = `${props.taskId}:`
	for (const key of [...cachedSnapshots.keys()]) {
		if (!key.startsWith(prefix) || drafts.has(key)) continue
		await deleteTaskTraceDraft('progress', props.taskId, key.slice(prefix.length))
		cachedSnapshots.delete(key)
		changed = true
	}
	for (const [key, draft] of drafts) {
		if (!key.startsWith(prefix)) continue
		const signature = JSON.stringify(draft)
		if (cachedSnapshots.get(key) === signature) continue
		await writeTaskTraceDraft('progress', props.taskId, key.slice(prefix.length), {progress: draft.html, references: draft.references, images: []})
		cachedSnapshots.set(key, signature)
		changed = true
	}
	if (changed) message.value = '草稿已自动缓存到 .cache，内容尚未保存；点击“保存进展”后才会正式提交。'
}

async function switchDate(value: string, initial = false) {
	if (saving.value || referenceLoading.value) {
		message.value = '正在保存或读取引用，请完成后再切换日期。'
		return false
	}
	if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
	if (!initial) stash()
	const request = ++version
	const taskId = props.taskId
	restoring.value = true
	date.value = value
	message.value = ''
	let loaded = false
	try {
		const history = await readTaskHistory(taskId)
		if (request !== version || taskId !== props.taskId) return false
		referenceHistory.value = history
		showReferencePicker.value = false
		selectedReferenceDates.value = []
		const selected = mergedDay(history, value, currentUsername.value)
		autoCommentId.value = selected.id
		autoTeamId.value = selected.teamId
		mergedIds.value = selected.mergedIds
		mergedTeamIds.value = selected.mergedTeamIds
		progress.value = selected.html
		references.value = normalizeProgressReferences(selected.references)
		const key = draftKey(taskId, value)
		let draft = drafts.get(key)
		if (!draft) {
			try {
				const cached = await readTaskTraceDraft<CachedProgressDraft>('progress', taskId, value)
				const legacy = JSON.parse(localStorage.getItem(`tasktrace-progress-draft-${taskId}`) || 'null')
				const saved = cached || (legacy?.date === value ? legacy : null)
				if (saved?.progress !== undefined) {
					draft = {html: saved.progress, references: normalizeProgressReferences(saved.references)}
					cachedSnapshots.set(key, JSON.stringify(draft))
				}
			} catch {
				message.value = '草稿恢复失败，已保留服务器内容。'
			}
		}
		if (request !== version || taskId !== props.taskId) return false
		lastSaved.value = snapshot()
		if (draft) {
			progress.value = draft.html
			references.value = normalizeProgressReferences(draft.references)
		}
		loaded = true
		message.value = draft
			? '已恢复 .cache 中的草稿，内容尚未保存；点击“保存进展”后才会正式提交。'
			: selected.id ? '已载入我在当天的最终进展；保存后评论区会保留修改记录。' : '我在此日期尚无进展。'
	} catch {
		message.value = '历史读取失败，已暂停保存，请重新选择日期重试。'
		return false
	} finally {
		if (request === version) restoring.value = !loaded
	}
	return loaded
}

async function memberSaved() {
	try {
		referenceHistory.value = await readTaskHistory(props.taskId)
	} finally {
		emit('saved')
	}
}

async function refreshHistory() {
	const request = ++historyRefreshVersion
	const taskId = props.taskId
	try {
		const history = await readTaskHistory(taskId)
		if (request !== historyRefreshVersion || taskId !== props.taskId) return false
		referenceHistory.value = history
		return true
	} catch {
		return false
	}
}

watch(() => props.taskId, async () => {
	if (isLocalBuild && !teamStore.loaded) {
		try { await teamStore.refresh() } catch { /* Personal progress remains available while collaboration status is unavailable. */ }
	}
	const now = new Date()
	const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
	await switchDate(today, true)
}, {immediate: true})

watch([progress, references], () => {
	stash()
	if (!restoring.value && snapshot() !== lastSaved.value) message.value = '内容尚未保存；自动保存只会缓存到 .cache，点击“保存进展”后才会正式提交。'
}, {deep: true})

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
	} catch {
		message.value = '历史进展读取失败，请重试。'
	} finally {
		referenceLoading.value = false
	}
}

function cancelReferencePicker() {
	showReferencePicker.value = false
	selectedReferenceDates.value = []
}

async function confirmReferences() {
	if (saving.value || restoring.value || referenceLoading.value || selectedReferenceDates.value.length === 0) return
	const taskId = props.taskId
	const request = version
	const selectedDates = [...selectedReferenceDates.value]
	referenceLoading.value = true
	try {
		const history = await readTaskHistory(taskId)
		if (request !== version || taskId !== props.taskId) return
		referenceHistory.value = history
		let added = 0
		for (const sourceDate of selectedDates) {
			if (references.value.some(item => item.date === sourceDate)) continue
			const reference = createProgressReference(taskId, sourceDate, date.value, mergedDay(history, sourceDate))
			if (reference) {
				references.value.push(reference)
				added++
			}
		}
		showReferencePicker.value = false
		selectedReferenceDates.value = []
		if (added) {
			referencesExpanded.value = true
			message.value = `已添加 ${added} 条引用，请在今日进展中填写更正说明。`
		} else {
			message.value = '所选日期已无可引用的进展，请重新选择。'
		}
	} catch {
		message.value = '引用读取失败，现有内容已保留，请重试。'
	} finally {
		referenceLoading.value = false
	}
}

async function save() {
	if (undoInProgress.value || restoring.value || saving.value || referenceLoading.value || sharedBusy.value || !canSave.value) return
	saving.value = true
	const taskId = props.taskId
	try {

		const latestHistory = await readTaskHistory(taskId)
		const outstanding = sharedOutstanding(latestHistory)
		if (!outstanding.id && outstanding.items.length > 0) await changeOutstanding(taskId, items => items, undoGroupHeaders())
		const body = await persistProgressImages(progress.value, taskId)
		const author = currentUsername.value
		const latestSelected = mergedDay(latestHistory, date.value, author)
		const absorbedIds = [...new Set([...mergedIds.value, ...latestSelected.mergedIds, autoCommentId.value, latestSelected.id].filter((id): id is number => !!id))]
		const absorbedTeamIds = [...new Set([...mergedTeamIds.value, ...latestSelected.mergedTeamIds, autoTeamId.value, latestSelected.teamId].filter(Boolean))]
		const numericAttribute = absorbedIds.length ? ` data-tasktrace-merged="${absorbedIds.join(',')}"` : ''
		const teamAttribute = absorbedTeamIds.length ? ` data-tasktrace-team-merged="${absorbedTeamIds.join(',')}"` : ''
		let comment = `<h3${numericAttribute}${teamAttribute}>每日进展 · ${date.value}</h3>${body}${serializeProgressReferences(references.value)}`
		if (snapshot() !== lastSaved.value || mergedIds.value.length || mergedTeamIds.value.length) {
			if (collaborative.value) {
				autoTeamId.value = createTeamCommentId()
				comment += serializeTeamCommentMarker({id: autoTeamId.value, author})
				autoCommentId.value = (await taskCommentsCreate({path: {task: taskId}, body: {comment}, headers: undoGroupHeaders()})).data.id
				mergedIds.value = absorbedIds
				mergedTeamIds.value = absorbedTeamIds
			} else if (autoCommentId.value) {
				await taskCommentsUpdate({path: {task: taskId, commentid: autoCommentId.value}, body: {comment}, headers: undoGroupHeaders()})
			} else {
				autoCommentId.value = (await taskCommentsCreate({path: {task: taskId}, body: {comment}, headers: undoGroupHeaders()})).data.id
			}
		}
		if (taskId !== props.taskId) return
		progress.value = body
		mergedIds.value = []
		mergedTeamIds.value = []
		lastSaved.value = snapshot()
		const key = draftKey(taskId, date.value)
		drafts.delete(key)
		cachedSnapshots.delete(key)
		await deleteTaskTraceDraft('progress', taskId, date.value).catch(() => {})
		try {
			localStorage.removeItem(`tasktrace-progress-draft-${taskId}`)
			localStorage.removeItem(`tasktrace-day-draft-${taskId}-${date.value}`)
		} catch { /* Remove legacy browser drafts. */ }
		referenceHistory.value = await readTaskHistory(taskId)
		message.value = '当天进展已正式保存，可继续修改。'
		emit('saved')
	} catch (cause) {
		const status = (cause as {response?: {status?: number}})?.response?.status
		message.value = status === 403
			? '当前协作权限为只读，内容已保留。所属人开放写权限后可直接再次保存。'
			: '保存失败，内容已保留，请重试。'
	} finally {
		saving.value = false
	}
}

defineExpose({switchDate, refreshHistory})
useAutoSave(async () => {
	if (!restoring.value && !sharedBusy.value) await cacheChangedDrafts()
})
onBeforeUnmount(() => {
	++version
	++historyRefreshVersion
	stash()
	if (autoSaveSettings.enabled) void cacheChangedDrafts().catch(() => {})
})
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

.daily-progress__heading-row {
	display: flex;
	align-items: start;
	justify-content: space-between;
	flex-wrap: wrap;
	gap: .65rem;

	p {
		margin: .2rem 0 0;
		color: var(--grey-600);
		font-size: .8125rem;
	}
}

.daily-progress__editor {
	min-inline-size: 0;
}

.daily-progress-editors {
	display: grid;
	gap: .65rem;
	min-inline-size: 0;
}

.progress-image-count,
.reference-hint {
	margin: 0;
	color: var(--grey-600);
	font-size: .875rem;
}

.member-progress-list {
	display: grid;
	gap: .75rem;

	h4 { margin: .25rem 0 0; }
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
	background: var(--white);
	color: var(--text);

	th,
	td {
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
	color: var(--text);
	overflow: auto;

	h2 {
		margin-block: 0 .5rem;
		color: var(--text-strong);
	}
}

.reference-dialog__actions {
	display: flex;
	justify-content: flex-end;
	gap: .65rem;
	margin-block-start: 1rem;
}

.daily-progress-actions {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: .75rem;

	span { font-size: .875rem; }
}
</style>
