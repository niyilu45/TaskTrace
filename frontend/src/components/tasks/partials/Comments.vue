<template>
	<div
		v-if="enabled"
		ref="commentsRef"
		class="content details comments-container"
	>
		<DailyProgress
			v-if="canWrite"
			:key="taskId"
			ref="dailyProgress"
			:task-id="taskId"
			@saved="dailyProgressSaved"
		/>
		<h2
			v-if="canWrite || comments.length > 0"
			class="comments-heading task-section-title"
			:class="{'d-print-none': comments.length === 0}"
		>
			<span>
				<span class="icon is-grey">
					<Icon :icon="['far', 'comments']" />
				</span>
				{{ $t('task.comment.title') }}
			</span>
			<BaseButton
				v-if="comments.length > 0"
				class="comment-sort-button"
				@click="toggleSortOrder"
			>
				<Icon :icon="commentSortOrder === 'asc' ? 'arrow-down-short-wide' : 'arrow-up-short-wide'" />
				{{ commentSortOrder === 'asc' ? $t('task.comment.sortOldestFirst') : $t('task.comment.sortNewestFirst') }}
			</BaseButton>
			<label
				v-if="commentAuthors.length > 1"
				class="comment-author-filter"
			>
				<span>筛选用户</span>
				<select v-model="selectedAuthor" class="input">
					<option value="">全部用户</option>
					<option v-for="author in commentAuthors" :key="author" :value="author">{{ author }}</option>
				</select>
			</label>
		</h2>
		<div class="comments">
			<p
				v-if="sourceMessage"
				role="status"
			>
				{{ sourceMessage }}
			</p>
			<div
				v-if="linkedComment && !comments.some(item => item.id === linkedComment?.id)"
				:id="`comment-${linkedComment.id}`"
				class="media comment linked-comment"
			>
				<div class="media-content">
					<strong>引用的原记录</strong>
					<ReadonlyRichText :html="linkedComment.comment" />
				</div>
			</div>
			<span
				v-if="taskCommentService.loading && saving === null && !creating"
				class="is-flex is-align-items-center mbs-4 mbe-4 mis-2"
			>
				<span class="loader is-inline-block mie-2" />
				{{ $t('task.comment.loading') }}
			</span>
			<div
				v-for="c in filteredComments"
				:id="`comment-${c.id}`"
				:key="c.id"
				class="media comment"
			>
				<figure class="media-left is-hidden-mobile">
					<UserAvatar
						:user="c.author"
						:size="48"
						class="image is-avatar"
					/>
					<figcaption class="is-sr-only">
						{{ $t('misc.avatarOfUser', {user: getDisplayName(c.author)}) }}
					</figcaption>
				</figure>
				<div class="media-content">
					<div class="comment-info">
						<UserAvatar
							:user="c.author"
							:size="20"
							class="image is-avatar d-print-none"
						/>
						<strong>{{ commentAuthor(c) }}</strong>
						<span
							v-tooltip="formatDateLong(c.created)"
							class="has-text-grey"
						>
							{{ formatDisplayDate(c.created) }}
						</span>
						<span
							v-if="+new Date(c.created) !== +new Date(c.updated)"
							v-tooltip="formatDateLong(c.updated)"
						>
							· {{ $t('task.comment.edited', {date: formatDisplayDate(c.updated)}) }}
						</span>
						<a
							v-tooltip="$t('task.comment.permalink')"
							:href="`#comment-${c.id}`"
							class="comment-permalink"
							:title="$t('task.comment.permalink')"
							@click.prevent.stop="copy(getCommentUrl(`${c.id}`))"
						>
							<span class="is-sr-only">{{ $t('task.comment.permalink') }}</span>
							<Icon icon="link" />
						</a>
						<CustomTransition name="fade">
							<span
								v-if="
									taskCommentService.loading &&
										saving === c.id
								"
								class="is-inline-flex"
							>
								<span class="loader is-inline-block mie-2" />
								{{ $t('misc.saving') }}
							</span>
							<span
								v-else-if="
									!taskCommentService.loading &&
										saved === c.id
								"
								class="has-text-success"
							>
								{{ $t('misc.saved') }}
							</span>
						</CustomTransition>
					</div>
					<template v-if="referencedDates[c.id]">
						<ReadonlyRichText :html="c.comment" />
						<div
							v-if="canWrite && commentOwnedByCurrent(c)"
							class="reference-comment-actions d-print-none"
						>
							<button
								type="button"
								class="button is-small"
								@click="editDailyProgress(referencedDates[c.id])"
							>
								编辑当天进展
							</button>
							<button
								type="button"
								class="button is-small"
								@click="toggleDelete(c.id)"
							>
								{{ $t('misc.delete') }}
							</button>
						</div>
					</template>
					<Editor
						v-else
						v-model="c.comment"
						:is-edit-enabled="canWrite && commentOwnedByCurrent(c)"
						:upload-callback="attachmentUpload"
						:upload-enabled="true"
						:bottom-actions="actions[c.id]"
						:show-save="true"
						:enable-discard-shortcut="true"
						:enable-mentions="true"
						:project-id="projectId"
						initial-mode="preview"
						@update:modelValue="
							() => {
								toggleEdit(c)
								editCommentWithDelay()
							}
						"
						@save="() => {
							toggleEdit(c)
							editComment()
						}"
					/>
					<ProgressBacklinks :items="progressBacklinkMap[c.id] || []" />
					<Reactions 
						v-model="c.reactions"
						class="mbs-2 d-print-none"
						entity-kind="comments"
						:entity-id="c.id"
						:disabled="!canWrite"
					/>
				</div>
			</div>

			<PaginationEmit
				v-if="taskCommentService.totalPages > 1"
				:total-pages="taskCommentService.totalPages"
				:current-page="currentPage"
				@pageChanged="changePage"
			/>

			<div
				v-if="canWrite"
				class="media comment d-print-none"
				:class="{'new-comment-top': commentSortOrder === 'desc'}"
			>
				<figure class="media-left is-hidden-mobile">
					<UserAvatar
						:user="authStore.info"
						:size="48"
						class="image is-avatar"
					/>
					<figcaption class="is-sr-only">
						{{ $t('misc.avatarOfUser', {user: getDisplayName(authStore.info)}) }}
					</figcaption>
				</figure>
				<div class="media-content">
					<div class="form">
						<CustomTransition name="fade">
							<span
								v-if="taskCommentService.loading && creating"
								class="is-inline-flex"
							>
								<span class="loader is-inline-block mie-2" />
								{{ $t('task.comment.creating') }}
							</span>
						</CustomTransition>
						<div class="field">
							<Editor
								v-if="editorActive"
								ref="newCommentEditor"
								v-model="newCommentText"
								:class="{
									'is-loading':
										taskCommentService.loading &&
										!isCommentEdit,
								}"
								:upload-callback="attachmentUpload"
								:placeholder="$t('task.comment.placeholder')"
								:enable-mentions="true"
								:project-id="projectId"
								:storage-key="commentStorageKey"
								@save="addComment()"
							/>
						</div>
						<div class="field">
							<XButton
								:loading="taskCommentService.loading && !isCommentEdit"
								:disabled="newCommentText === ''"
								@click="addComment()"
							>
								{{ $t('task.comment.comment') }}
							</XButton>
						</div>
					</div>
				</div>
			</div>
		</div>


		<Modal
			:enabled="showDeleteModal"
			@close="showDeleteModal = false"
			@submit="() => deleteComment(commentToDelete)"
		>
			<template #header>
				<span>{{ $t('task.comment.delete') }}</span>
			</template>

			<template #text>
				<p>
					{{ $t('task.comment.deleteText1') }}<br>
					<strong class="has-text-white">{{ isLocalBuild ? '删除后可使用“撤销”恢复。' : $t('misc.cannotBeUndone') }}</strong>
				</p>
			</template>
		</Modal>
	</div>
</template>

<script setup lang="ts">
import {ref, reactive, computed, nextTick, provide, shallowReactive, watch, onBeforeUnmount} from 'vue'
import {useTasktraceUndoGuard, undoInProgress} from '@/helpers/tasktraceUndo'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {isEditorContentEmpty} from '@/helpers/editorContentEmpty'
import {useI18n} from 'vue-i18n'
import {useRoute} from 'vue-router'
import {taskCommentsRead, type TaskComment} from '@/client/generated'
import {parseProgressNote, progressBacklinks} from '@/helpers/progressNotes'
import ReadonlyRichText from './ReadonlyRichText.vue'
import ProgressBacklinks from './ProgressBacklinks.vue'

import BaseButton from '@/components/base/BaseButton.vue'
import CustomTransition from '@/components/misc/CustomTransition.vue'
import Editor from '@/components/input/AsyncEditor'
import PaginationEmit from '@/components/misc/PaginationEmit.vue'
import UserAvatar from '@/components/misc/UserAvatar.vue'

import TaskCommentService from '@/services/taskComment'
import TaskCommentModel from '@/models/taskComment'

import type {ITaskComment} from '@/modelTypes/ITaskComment'
import type {ITask} from '@/modelTypes/ITask'

import {uploadFile, uploadFilesForEditor} from '@/helpers/attachments'
import {success} from '@/message'
import {formatDateLong, formatDisplayDate} from '@/helpers/time/formatDate'
import {clearEditorDraft} from '@/helpers/editorDraftStorage'
import {getDisplayName} from '@/models/user'
import DailyProgress from './DailyProgress.vue'
import {useConfigStore} from '@/stores/config'
import {useAuthStore} from '@/stores/auth'
import Reactions from '@/components/input/Reactions.vue'
import {useCopyToClipboard} from '@/composables/useCopyToClipboard'
import {commentReplyContextKey, scrollAndHighlightComment} from '@/components/tasks/partials/commentReplyContext'
import {readTeamCommentMarker, teamCommentAuthor} from '@/helpers/tasktraceTeam'

const props = withDefaults(defineProps<{
	taskId: number,
	projectId: number,
	canWrite?: boolean
	initialComments: ITaskComment[]
}>(), {
	canWrite: true,
})

const copy = useCopyToClipboard()
const route = useRoute()
const dailyProgress = ref<InstanceType<typeof DailyProgress> | null>(null)
const linkedComment = ref<TaskComment | null>(null)
const sourceMessage = ref('')
let sourceRequest = 0
const referencedDates = computed<Record<number, string>>(() => Object.fromEntries(comments.value.flatMap(comment => {
	const note = parseProgressNote({comment: comment.comment})
	return note.daily && note.references.length ? [[comment.id, note.date]] : []
})))
const progressBacklinkMap = computed(() => progressBacklinks(comments.value as unknown as TaskComment[]))
async function editDailyProgress(date: string) {
	if (!await dailyProgress.value?.switchDate(date)) return
	const input = document.getElementById(`progress-text-${props.taskId}`)
	input?.scrollIntoView({block: 'center', behavior: 'smooth'})
	input?.focus({preventScroll: true})
}
async function revealSourceComment() {
	const request = ++sourceRequest
	const id = Number(route.hash.match(/^#comment-([1-9]\d*)$/)?.[1])
	linkedComment.value = null; sourceMessage.value = ''
	if (!Number.isSafeInteger(id) || id < 1 || !enabled.value) return
	try {
		if (!comments.value.some(comment => comment.id === id)) {
			sourceMessage.value = '正在读取原记录…'
			const result = await taskCommentsRead({path: {task: props.taskId, commentid: id}})
			if (request !== sourceRequest) return
			linkedComment.value = result.data
		}
		sourceMessage.value = ''
		await nextTick()
		if (request === sourceRequest) scrollAndHighlightComment(id)
	} catch {
		if (request === sourceRequest) sourceMessage.value = '原记录暂时无法读取，可能已删除或无访问权限；引用中的快照仍可查看。'
	}
}

const {t} = useI18n({useScope: 'global'})
const configStore = useConfigStore()
const authStore = useAuthStore()

const localSortOrder = ref<'asc' | 'desc' | null>(null)
const commentSortOrder = computed(() => localSortOrder.value ?? authStore.settings.frontendSettings.commentSortOrder ?? 'asc')

const comments = ref<ITaskComment[]>([])
const selectedAuthor = ref('')
const commentAuthor = (comment: ITaskComment) => teamCommentAuthor(comment.comment || '', getDisplayName(comment.author))
const commentOwnedByCurrent = (comment: ITaskComment) => {
	const marker = readTeamCommentMarker(comment.comment || '')
	return marker ? marker.author.toLowerCase() === (authStore.info?.username || '').toLowerCase() : comment.author.id === currentUserId.value
}
const commentAuthors = computed(() => [...new Set(comments.value.map(commentAuthor).filter(Boolean))].sort((a, b) => a.localeCompare(b)))
const filteredComments = computed(() => selectedAuthor.value ? comments.value.filter(comment => commentAuthor(comment) === selectedAuthor.value) : comments.value)
const savedComments = reactive(new Map<number, string>())
const uploading = ref(0)
function rememberComments() {
	savedComments.clear()
	comments.value.forEach(comment => savedComments.set(comment.id, comment.comment))
	void revealSourceComment()
}

const showDeleteModal = ref(false)
const commentToDelete = reactive(new TaskCommentModel())

const isCommentEdit = ref(false)
const commentEdit = reactive(new TaskCommentModel())

const newCommentText = ref('')

const saved = ref<ITask['id'] | null>(null)
const saving = ref<ITask['id'] | null>(null)

const currentUserId = computed(() => authStore.info?.id)
const enabled = computed(() => configStore.taskCommentsEnabled)
const actions = computed(() => {
	if (!props.canWrite) {
		return {}
	}
	return Object.fromEntries(comments.value.map((comment) => {
		const list: {action: () => void, title: string}[] = [{
			action: () => startReplyTo(comment),
			title: t('task.comment.reply'),
		}]
		if (commentOwnedByCurrent(comment)) {
			list.push({
				action: () => toggleDelete(comment.id),
				title: t('misc.delete'),
			})
		}
		return [comment.id, list]
	}))
})

const frontendUrl = computed(() => configStore.frontendUrl)
const commentStorageKey = computed(() => `task-comment-${props.taskId}`)

const currentPage = ref(1)

const commentsRef = ref<HTMLElement | null>(null)
const newCommentEditor = ref<{setReplyContent: (html: string) => Promise<void>} | null>(null)

provide(commentReplyContextKey, {
	findComment: (id: number) => comments.value.find(c => c.id === id),
	scrollToComment: scrollAndHighlightComment,
})

// Strip <mention-user> elements from a reply quote so reposting the parent
// body doesn't trigger fresh notifications for users mentioned in the
// original. The inner text is kept so the quote still reads correctly.
function stripMentionsForQuote(html: string): string {
	if (!html) {
		return ''
	}
	const doc = new DOMParser().parseFromString(`<div>${html}</div>`, 'text/html')
	doc.querySelectorAll('mention-user').forEach((el) => {
		const label = (el.getAttribute('data-label') ?? el.textContent ?? '').trim()
		el.replaceWith(label ? `@${label.replace(/^@+/, '')}` : '')
	})
	return doc.body.firstElementChild?.innerHTML ?? ''
}

async function startReplyTo(parent: ITaskComment) {
	const body = stripMentionsForQuote(parent.comment ?? '')
	const draft = `<blockquote data-comment-id="${parent.id}">${body}</blockquote><p></p>`
	if (!editorActive.value) {
		editorActive.value = true
	}
	// Editor mounts asynchronously through defineAsyncComponent; wait until
	// the ref is populated before pushing content in. Bail with a warning
	// rather than fall back to `newCommentText = draft` — the modelValue
	// watcher in TipTap.vue would land the editor in preview mode, leaving
	// the user unable to type without clicking the editor first.
	const editor = await waitForEditorRef()
	if (!editor) {
		console.warn('Reply editor did not mount in time; aborting reply prefill.')
		return
	}
	await editor.setReplyContent(draft)
}

async function waitForEditorRef() {
	const start = performance.now()
	while (!newCommentEditor.value && performance.now() - start < 2000) {

		await nextTick()
	}
	return newCommentEditor.value
}


async function attachmentUpload(files: File[] | FileList): Promise<string[]> {
	uploading.value++
	try {
		return await uploadFilesForEditor((file, onSuccess) => uploadFile(props.taskId, file, onSuccess), files)
	} finally { uploading.value-- }
}

const taskCommentService = shallowReactive(new TaskCommentService())

async function dailyProgressSaved() {
	localSortOrder.value = 'desc'
	currentPage.value = 1
	await loadComments(props.taskId, true)
}

async function loadComments(taskId: ITask['id'], force = false) {
	if (!enabled.value) {
		return
	}

	if (currentPage.value === 1) {
		taskCommentService.totalPages = 0
		taskCommentService.resultCount = 0
	}

	commentEdit.taskId = taskId
	commentToDelete.taskId = taskId

	if (!force && commentSortOrder.value === 'asc' && typeof props.initialComments !== 'undefined' && currentPage.value === 1) {
		if (props.initialComments.length < configStore.maxItemsPerPage) {
			comments.value = props.initialComments
			rememberComments()
			return
		}
	}

	comments.value = await taskCommentService.getAll({taskId}, {order_by: commentSortOrder.value}, currentPage.value)
	rememberComments()
}

async function changePage(page: number) {
	commentsRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start', inline: 'nearest' })
	currentPage.value = page
	await loadComments(props.taskId)
}

async function toggleSortOrder() {
	const newOrder = commentSortOrder.value === 'asc' ? 'desc' : 'asc'
	if (!authStore.isLinkShareAuth) {
		await authStore.saveUserSettings({
			settings: {
				...authStore.settings,
				frontendSettings: {
					...authStore.settings.frontendSettings,
					commentSortOrder: newOrder,
					quickAddDefaultReminders: [...(authStore.settings.frontendSettings.quickAddDefaultReminders ?? [])],
				},
			},
			showMessage: false,
		})
	} else {
		localSortOrder.value = newOrder
	}
	if (taskCommentService.totalPages > 1) {
		currentPage.value = 1
		await loadComments(props.taskId)
	} else {
		comments.value.reverse()
	}
}

watch(
	() => [props.taskId, props.initialComments],
	() => {
		currentPage.value = 1 // Reset to first page when task changes
		loadComments(props.taskId)
	},
	{immediate: true},
)

watch(() => route.hash, revealSourceComment)

const editorActive = ref(true)
const creating = ref(false)

async function addComment() {
	if (undoInProgress.value || newCommentText.value === '') {
		return
	}

	creating.value = true

	try {
		const newComment = new TaskCommentModel()
		newComment.taskId = props.taskId
		newComment.comment = newCommentText.value
		const comment = await taskCommentService.create(newComment)
		savedComments.set(comment.id, comment.comment)

		if (commentSortOrder.value === 'desc' && currentPage.value > 1) {
			currentPage.value = 1
			await loadComments(props.taskId)
		} else if (commentSortOrder.value === 'desc') {
			comments.value.unshift(comment)
		} else {
			comments.value.push(comment)
		}
		newCommentText.value = ''

		// Ensure draft is cleared from localStorage
		clearEditorDraft(commentStorageKey.value)

		if (commentSortOrder.value === 'desc') {
			commentsRef.value?.scrollIntoView({behavior: 'smooth', block: 'start', inline: 'nearest'})
		}

		success({message: t('task.comment.addedSuccess')})
	} finally {
		creating.value = false
	}
}

function toggleEdit(comment: ITaskComment) {
	isCommentEdit.value = !isCommentEdit.value
	Object.assign(commentEdit, comment)
}

function toggleDelete(commentId: ITaskComment['id']) {
	showDeleteModal.value = !showDeleteModal.value
	commentToDelete.id = commentId
}

const changeTimeout = ref<ReturnType<typeof setTimeout> | null>(null)
let disposed = false
useTasktraceUndoGuard(() => !isEditorContentEmpty(newCommentText.value) || uploading.value > 0 || creating.value || saving.value !== null || changeTimeout.value !== null || comments.value.some(comment => savedComments.get(comment.id) !== comment.comment), '请先保存或清空评论草稿，再撤销。')
onBeforeUnmount(() => {
	if (changeTimeout.value !== null) {
		clearTimeout(changeTimeout.value)
		changeTimeout.value = null
		if (!undoInProgress.value) void editComment()
	}
	sourceRequest++
	disposed = true
})

async function editCommentWithDelay() {
	if (changeTimeout.value !== null) {
		clearTimeout(changeTimeout.value)
	}

	changeTimeout.value = setTimeout(async () => {
		await editComment()
	}, 5000)
}

async function editComment() {
	if (changeTimeout.value !== null) {
		clearTimeout(changeTimeout.value)
		changeTimeout.value = null
	}
	if (undoInProgress.value || disposed || commentEdit.comment === '') return
	const submitted = new TaskCommentModel({...commentEdit, taskId: props.taskId})
	saving.value = submitted.id
	try {
		const comment = await taskCommentService.update(submitted)
		for (let c = 0; c < comments.value.length; c++) {
			if (comments.value[c].id === submitted.id) {
				comments.value[c] = comment
			}
		}
		savedComments.set(comment.id, comment.comment)
		saved.value = submitted.id
		setTimeout(() => {
			saved.value = null
		}, 2000)
	} finally {
		isCommentEdit.value = false
		saving.value = null
	}
}

async function deleteComment(commentToDelete: ITaskComment) {
	try {
		await taskCommentService.delete(commentToDelete)
		const index = comments.value.findIndex(({id}) => id === commentToDelete.id)
		comments.value.splice(index, 1)
		success({message: t('task.comment.deleteSuccess')})
	} finally {
		showDeleteModal.value = false
	}
}

function getCommentUrl(commentId: string) {
	const baseUrl = frontendUrl.value.endsWith('/') ? frontendUrl.value.slice(0, -1) : frontendUrl.value
	const url = new URL(location.pathname + location.search, baseUrl)
	url.hash = `comment-${commentId}`
	return url.toString()
}
</script>

<style lang="scss" scoped>
.reference-comment-actions {
	display: flex;
	flex-wrap: wrap;
	gap: .5rem;
	margin-block-start: .5rem;
}

.media {
	align-items: flex-start;
	display: flex;
	text-align: inherit;
	padding-block-start: .5rem;

	& + .media {
		margin-block-start: .5rem;
	}
}

.media-left {
	flex-basis: auto;
	flex-grow: 0;
	flex-shrink: 0;
	margin: 0 .5rem !important;
}

.comment-info {
	display: flex;
	align-items: center;
	gap: .5rem;

	img {
		@media screen and (max-width: $tablet) {
			display: block;
			inline-size: 20px;
			block-size: 20px;
			padding-inline-end: 0;
			margin-inline-end: .5rem;
		}

		@media screen and (min-width: $tablet) {
			display: none;
		}
	}


	span,
	.comment-permalink {
		font-size: .75rem;
		line-height: 1;
	}

	.comment-permalink {
		font-size: 1rem;
		border: 1px solid transparent;
		padding: 0.25rem;
		border-radius: 1rem;
		color: var(--grey, hsl(0, 0%, 48%));
	}
	.comment-permalink:hover {
		color: var(--grey-dark, hsl(0, 0%, 29%));
		border-color: var(--grey-dark, hsl(0, 0%, 29%));
	}
}

.image.is-avatar {
	border-radius: 100%;
}

.media-content {
	flex-basis: auto;
	flex-grow: 1;
	flex-shrink: 1;
	text-align: inherit;
	inline-size: calc(100% - 48px - 2rem);
}

.comments-heading {
	display: flex;
	align-items: center;
	justify-content: flex-start;
	gap: .75rem;
	flex-wrap: wrap;

	> :first-child { margin-inline-end: auto; }
}

.comment-author-filter {
	display: inline-flex;
	align-items: center;
	gap: .4rem;
	font-size: .75rem;
	font-weight: 400;

	.input { min-inline-size: 8rem; block-size: 2rem; }
}

.comment-sort-button {
	font-size: .75rem;
	font-weight: normal;
	color: var(--grey-500);
	display: inline-flex;
	align-items: center;
	gap: .25rem;

	&:hover {
		color: var(--grey-700);
	}
}

.comments {
	display: flex;
	flex-direction: column;
}

.new-comment-top {
	order: -1;
}

.comments-container {
	scroll-margin-block-start: 4rem;
}

.media.comment {
	scroll-margin-block-start: 4rem;
	transition: background-color .3s ease-out;
	border-radius: $radius;
}

.media.comment.comment-highlight {
	background-color: hsla(var(--primary-hsl), 0.18);
	transition: background-color .15s ease-in;
}
</style>
