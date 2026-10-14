<template>
	<XButton
		variant="secondary"
		icon="download"
		@click="open = true"
	>
		导出 Markdown
	</XButton>
	<Modal
		:enabled="open"
		submit-label="选择位置并保存"
		@close="close"
		@submit="save"
	>
		<template #header>
			导出任务
		</template>
		<template #text>
			<p><strong>{{ title }}</strong></p>
			<p>导出已保存的任务描述、每日进展、遗留事项和可读取的子任务。图片内嵌在文件中，离线也可预览。</p>
			<p class="help">
				默认文件名：{{ markdownFilename(title) }}。部分 Markdown 平台会限制内嵌图片；可在本程序预览完整内容。
			</p>
			<p
				v-if="!hasFilePicker"
				class="help"
			>
				当前浏览器将通过下载保存；保存位置由浏览器的下载设置决定。
			</p>
			<XButton
				variant="secondary"
				icon="eye"
				@click="preview"
			>
				预览（新标签页）
			</XButton>
			<p
				v-if="message"
				role="status"
			>
				{{ message }}
			</p>
		</template>
	</Modal>
</template>

<script setup lang="ts">
import {ref} from 'vue'
import {useRouter} from 'vue-router'
import XButton from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import {markdownFilename, saveTaskMarkdown} from '@/helpers/taskMarkdown'
const props = defineProps<{taskId: number, title: string}>()
const router = useRouter()
const hasFilePicker = 'showSaveFilePicker' in window
const open = ref(false), saving = ref(false), message = ref('')
function close() {if (!saving.value) open.value = false}
function preview() {window.open(router.resolve({name: 'task.markdown', params: {id: props.taskId}}).href, '_blank', 'noopener')}
async function save() {
	if (saving.value) return
	saving.value = true; message.value = '请选择保存位置…'
	try {const warnings = await saveTaskMarkdown(props.taskId, props.title); message.value = warnings.length ? `文件已保存。${warnings.join(' ')}` : 'Markdown 文件已保存。'}
	catch (error) {message.value = error instanceof Error && error.name === 'AbortError' ? '已取消导出。' : '导出失败，请重试。'}
	finally {saving.value = false}
}
</script>
