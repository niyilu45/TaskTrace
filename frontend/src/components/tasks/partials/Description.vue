<template>
	<div
		:class="{'d-print-none': isEmpty}"
	>
		<h2 class="task-section-title">
			<span class="icon is-grey">
				<Icon icon="align-left" />
			</span>
			{{ $t('task.attributes.description') }}
			<CustomTransition name="fade">
				<span
					v-if="saveState === 'saving'"
					class="is-small is-inline-flex"
					aria-hidden="true"
				>
					<span class="loader is-inline-block mie-2" />
					{{ $t('misc.saving') }}
				</span>
				<span
					v-else-if="saveState === 'saved'"
					class="is-small has-text-success"
					aria-hidden="true"
				>
					<Icon icon="check" />
					{{ $t('misc.saved') }}
				</span>
			</CustomTransition>
		</h2>
		<!-- Outside the h2 so the heading keeps a stable accessible name -->
		<span
			class="is-sr-only"
			role="status"
			aria-live="polite"
		>{{ saveStateAnnouncement }}</span>
		<Editor
			v-model="description"
			class="tiptap__task-description"
			:is-edit-enabled="canWrite"
			:upload-callback="uploadCallback"
			:placeholder="$t('task.description.placeholder')"
			:show-save="true"
			:show-discard="true"
			:start-in-edit-when-empty="false"
			edit-shortcut="KeyE"
			:enable-discard-shortcut="true"
			:enable-mentions="true"
			:project-id="modelValue.projectId"
			:storage-key="descriptionStorageKey"
			@update:modelValue="markChanged"
			@save="save"
			@discard="discard"
		/>
	</div>
</template>

<script setup lang="ts">
import {ref, computed, watch, onBeforeUnmount} from 'vue'
import {useTasktraceUndoGuard, undoInProgress} from '@/helpers/tasktraceUndo'
import {useI18n} from 'vue-i18n'

import CustomTransition from '@/components/misc/CustomTransition.vue'
import Editor from '@/components/input/AsyncEditor'

import { clearEditorDraft } from '@/helpers/editorDraftStorage'
import { isEditorContentEmpty } from '@/helpers/editorContentEmpty'
import { uploadFilesForEditor } from '@/helpers/attachments'
import type { ITask } from '@/modelTypes/ITask'
import { useTaskStore } from '@/stores/tasks'

export type AttachmentUploadFunction = (file: File, onSuccess: (attachmentUrl: string) => void) => Promise<string>

const props = defineProps<{
	modelValue: ITask,
	attachmentUpload: AttachmentUploadFunction,
	canWrite: boolean,
	ensureCanWrite?: () => Promise<boolean>,
}>()

const emit = defineEmits<{
	'update:modelValue': [value: ITask]
}>()

const description = ref<string>('')
const hasChanges = ref(false)
watch(() => props.modelValue.description, value => {
	if (!hasChanges.value) description.value = value
}, {immediate: true})
watch(() => props.modelValue.id, () => {
	description.value = props.modelValue.description
	hasChanges.value = false
})

const saved = ref(false)
const saving = ref(false)
const uploading = ref(0)
useTasktraceUndoGuard(() => hasChanges.value || saving.value || uploading.value > 0, '请先保存或取消事项描述的修改。')

const taskStore = useTaskStore()

const {t} = useI18n({useScope: 'global'})

const savedTimeout = ref<ReturnType<typeof setTimeout> | null>(null)
const dwellTimeout = ref<ReturnType<typeof setTimeout> | null>(null)

// Saves resolve faster than "Saving…" can be read, and aria-live coalesces it away, so hold it a floor.
const MIN_SAVING_DWELL = 500
const dwelling = ref(false)

watch(saving, isSaving => {
	if (!isSaving) {
		return
	}

	dwelling.value = true
	if (dwellTimeout.value !== null) {
		clearTimeout(dwellTimeout.value)
	}
	dwellTimeout.value = setTimeout(() => {
		dwelling.value = false
	}, MIN_SAVING_DWELL)
})

const saveState = computed(() => {
	if (saving.value || dwelling.value) {
		return 'saving'
	}

	if (saved.value) {
		return 'saved'
	}

	return ''
})

// Runs from when "Saved!" reaches the screen, not from the response the floor may have delayed.
watch(saveState, state => {
	if (savedTimeout.value !== null) {
		clearTimeout(savedTimeout.value)
	}

	if (state !== 'saved') {
		return
	}

	savedTimeout.value = setTimeout(() => {
		saved.value = false
	}, 2000)
})

const saveStateAnnouncement = computed(() => {
	if (saveState.value === 'saving') {
		return t('misc.saving')
	}

	if (saveState.value === 'saved') {
		return t('misc.saved')
	}

	return ''
})

const descriptionStorageKey = computed(() => `task-description-${props.modelValue.id}`)

const isEmpty = computed(() => isEditorContentEmpty(description.value))

function markChanged() {
	if (description.value === props.modelValue.description) {
		hasChanges.value = false
		return
	}

	hasChanges.value = true
}

function discard(savedDescription: string) {
	description.value = savedDescription
	hasChanges.value = false
}

onBeforeUnmount(() => {
	if (savedTimeout.value !== null) {
		clearTimeout(savedTimeout.value)
	}
	if (dwellTimeout.value !== null) {
		clearTimeout(dwellTimeout.value)
	}
})

async function save() {
	if (undoInProgress.value || !hasChanges.value || saving.value || !props.canWrite) {
		return
	}
	if (props.ensureCanWrite && !await props.ensureCanWrite()) return

	const submitted = description.value
	saved.value = false
	saving.value = true

	try {
		const updated = await taskStore.update({
			...props.modelValue,
			description: submitted,
		})
		hasChanges.value = description.value !== submitted
		emit('update:modelValue', updated)

		// Clear draft from localStorage when saved successfully
		if (!hasChanges.value) clearEditorDraft(descriptionStorageKey.value)

		saved.value = true
	} catch (error) {
		// If the task was deleted (404), silently skip saving
		if (error?.response?.status === 404) {
			return
		}
		hasChanges.value = true
		// Re-throw other errors
		throw error
	} finally {
		saving.value = false
	}
}

async function uploadCallback(files: File[] | FileList): Promise<string[]> {
	uploading.value++
	try { return await uploadFilesForEditor(props.attachmentUpload, files) } finally { uploading.value-- }
}
</script>

<style lang="scss" scoped>
.tiptap__task-description {
	// The exact amount of pixels we need to make the description icon align with the buttons and the form inside the editor.
	// The icon is not exactly the same length on all sides so we need to hack our way around it.
	margin-inline-start: 4px;
}
</style>
