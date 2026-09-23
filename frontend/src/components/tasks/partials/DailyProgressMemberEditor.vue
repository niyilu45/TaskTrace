<template>
	<form
		class="member-progress-editor"
		@submit.prevent="save"
	>
		<div class="member-progress-editor__heading">
			<div>
				<strong>{{ author }} 的进展</strong>
				<span>该成员在 {{ date }} 已提交进展，可共同修改；保存后保留修改记录。</span>
			</div>
			<button
				type="submit"
				class="button is-primary"
				:disabled="saving || restoring || unchanged"
			>
				{{ saving ? '正在保存…' : '保存此成员进展' }}
			</button>
		</div>
		<Editor
			v-model="progress"
			:always-editing="true"
			:allow-base64-images="true"
			:upload-callback="stageProgressImages"
			:placeholder="`修改 ${author} 在 ${date} 的进展`"
			@save="save"
		/>
		<p
			class="member-progress-editor__status"
			role="status"
		>
			{{ message || `正文内有 ${imageCount} 张图片；选中图片后可按退格或 Delete 删除。` }}
		</p>
	</form>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import type {TaskComment} from '@/client/generated'
import {taskCommentsCreate} from '@/client/generated'
import Editor from '@/components/input/AsyncEditor'
import {mergedDay} from '@/helpers/progressNotes'
import {createTeamCommentId, serializeTeamCommentMarker} from '@/helpers/tasktraceTeam'
import {serializeProgressReferences, normalizeProgressReferences, type ProgressReference} from '@/helpers/progressReferences'
import {countProgressImages, persistProgressImages, stageProgressImages} from '@/helpers/progressEditorImages'
import {deleteTaskTraceDraft, forgetRememberedTaskTraceDraft, readTaskTraceDraft, rememberTaskTraceDraft, writeTaskTraceDraft} from '@/helpers/tasktraceDraftCache'
import {autoSaveSettings, useAutoSave} from '@/helpers/autoSave'
import {undoGroupHeaders, undoInProgress, useTasktraceUndoGuard} from '@/helpers/tasktraceUndo'
import {readTaskHistory} from '@/helpers/sharedOutstanding'

const props = defineProps<{
	taskId: number,
	date: string,
	author: string,
	editor: string,
	history: TaskComment[],
}>()
const emit = defineEmits<{saved: []}>()

const progress = ref('')
const references = ref<ProgressReference[]>([])
const restoring = ref(true)
const saving = ref(false)
const message = ref('')
const loadedId = ref<number>()
const loadedTeamId = ref('')
const mergedIds = ref<number[]>([])
const mergedTeamIds = ref<string[]>([])
let pendingCreate: {identity: string, signature: string, id: string} | undefined
let version = 0

const imageCount = computed(() => countProgressImages(progress.value))
const snapshot = computed(() => JSON.stringify([progress.value, references.value]))
const savedSnapshot = ref('')
const unchanged = computed(() => snapshot.value === savedSnapshot.value)
const cacheDay = computed(() => `${props.date}--${encodeURIComponent(props.author.trim().toLowerCase()) || 'member'}`)
type CachedDraft = {progress?: string, references?: ProgressReference[]}

useTasktraceUndoGuard(() => saving.value || restoring.value || !unchanged.value, '请先保存协作成员的每日进展草稿。')

async function load() {
	const request = ++version
	restoring.value = true
	message.value = ''
	const selected = mergedDay(props.history, props.date, props.author)
	progress.value = selected.html
	references.value = normalizeProgressReferences(selected.references)
	loadedId.value = selected.id
	loadedTeamId.value = selected.teamId
	mergedIds.value = selected.mergedIds
	mergedTeamIds.value = selected.mergedTeamIds
	savedSnapshot.value = snapshot.value
	try {
		const draft = await readTaskTraceDraft<CachedDraft>('progress', props.taskId, cacheDay.value)
		if (request !== version) return
		if (draft?.progress !== undefined) {
			progress.value = draft.progress
			references.value = normalizeProgressReferences(draft.references)
			message.value = '已恢复 .cache 中的草稿，内容尚未保存。'
		}
	} catch {
		message.value = '草稿恢复失败，已保留服务器内容。'
	}
	if (request === version) restoring.value = false
}

watch(() => [props.taskId, props.date, props.author, props.history] as const, load, {immediate: true, deep: true})
watch(snapshot, () => {
	if (!restoring.value) {
		if (unchanged.value) forgetRememberedTaskTraceDraft('progress', props.taskId, cacheDay.value)
		else rememberTaskTraceDraft('progress', props.taskId, cacheDay.value, {progress: progress.value, references: normalizeProgressReferences(references.value), images: []})
	}
	if (!restoring.value && !unchanged.value) message.value = '内容尚未保存；自动保存只缓存到 .cache，点击保存后才会正式提交。'
})

function handleBeforeUnload(event: BeforeUnloadEvent) {
	if (restoring.value || unchanged.value) return
	rememberTaskTraceDraft('progress', props.taskId, cacheDay.value, {progress: progress.value, references: normalizeProgressReferences(references.value), images: []})
	event.preventDefault()
	event.returnValue = ''
}

async function cacheDraft() {
	if (restoring.value || unchanged.value) return
	await writeTaskTraceDraft('progress', props.taskId, cacheDay.value, {
		progress: progress.value,
		references: normalizeProgressReferences(references.value),
		images: [],
	})
	message.value = '草稿已自动缓存到 .cache，内容尚未保存；点击保存后才会正式提交。'
}

async function save() {
	if (undoInProgress.value || saving.value || restoring.value || unchanged.value) return
	saving.value = true
	try {
		const latest = await readTaskHistory(props.taskId)
		const selected = mergedDay(latest, props.date, props.author)
		const body = await persistProgressImages(progress.value, props.taskId)
		progress.value = body
		const identity = `${props.taskId}:${props.date}:${props.author.trim().toLowerCase()}`
		const signature = JSON.stringify([identity, body, normalizeProgressReferences(references.value)])
		if (!pendingCreate || pendingCreate.identity !== identity || pendingCreate.signature !== signature) {
			pendingCreate = {identity, signature, id: createTeamCommentId()}
		}
		const absorbedIds = [...new Set([...mergedIds.value, ...selected.mergedIds, loadedId.value, selected.id].filter((id): id is number => !!id))]
		const absorbedTeamIds = [...new Set([...mergedTeamIds.value, ...selected.mergedTeamIds, loadedTeamId.value, selected.teamId].filter(Boolean))]
		const numericAttribute = absorbedIds.length ? ` data-tasktrace-merged="${absorbedIds.join(',')}"` : ''
		const teamAttribute = absorbedTeamIds.length ? ` data-tasktrace-team-merged="${absorbedTeamIds.join(',')}"` : ''
		const teamId = pendingCreate.id
		const comment = `<h3${numericAttribute}${teamAttribute}>每日进展 · ${props.date}</h3>${body}${serializeProgressReferences(references.value)}${serializeTeamCommentMarker({id: teamId, author: props.author, editor: props.editor})}`
		const result = await taskCommentsCreate({path: {task: props.taskId}, body: {comment}, headers: undoGroupHeaders()})
		progress.value = body
		loadedId.value = result.data.id
		loadedTeamId.value = teamId
		pendingCreate = undefined
		mergedIds.value = absorbedIds
		mergedTeamIds.value = absorbedTeamIds
		savedSnapshot.value = snapshot.value
		await deleteTaskTraceDraft('progress', props.taskId, cacheDay.value).catch(() => {})
		message.value = `${props.author} 的当天进展已保存；其他成员的进展未改动。`
		emit('saved')
	} catch {
		message.value = '保存失败，内容已保留，请重试。'
	} finally {
		saving.value = false
	}
}

useAutoSave(async () => {
	if (autoSaveSettings.enabled) await cacheDraft()
})
onMounted(() => window.addEventListener('beforeunload', handleBeforeUnload))
onBeforeUnmount(() => {
	++version
	if (autoSaveSettings.enabled) void cacheDraft().catch(() => {})
	window.removeEventListener('beforeunload', handleBeforeUnload)
})
</script>

<style scoped lang="scss">
.member-progress-editor {
	display: grid;
	gap: .6rem;
	padding: .8rem;
	border: 1px solid var(--grey-200);
	border-radius: $radius;
	background: var(--grey-50);
}

.member-progress-editor__heading {
	display: flex;
	align-items: start;
	justify-content: space-between;
	gap: .75rem;

	div {
		display: grid;
		gap: .2rem;
	}

	span {
		color: var(--grey-600);
		font-size: .8125rem;
	}
}

.member-progress-editor__status {
	margin: 0;
	color: var(--grey-600);
	font-size: .8125rem;
}

@media (width <= 48rem) {
	.member-progress-editor__heading {
		align-items: stretch;
		flex-direction: column;
	}
}
</style>
