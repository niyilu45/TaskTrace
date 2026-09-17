<template>
	<section aria-label="共享遗留事项">
		<h4>遗留事项（所有日期共享）</h4>
		<p
			v-if="loading"
			role="status"
		>
			正在读取…
		</p>
		<ul>
			<li
				v-for="item in items"
				:key="item.id"
			>
				<ReadonlyRichText :html="item.html" />
				<button
					type="button"
					class="button is-small"
					:disabled="disabled || busy"
					@click="remove(item.id)"
				>
					移除
				</button>
			</li>
		</ul>
		<div class="field has-addons">
			<input
				v-model="text"
				class="input"
				aria-label="新增遗留事项"
				placeholder="输入一条遗留事项"
				:disabled="disabled || busy || loading"
				@keydown.enter.prevent="add"
			>
			<button
				type="button"
				class="button"
				:disabled="disabled || busy || loading || !text.trim()"
				@click="add"
			>
				添加遗留事项
			</button>
		</div>
		<p
			v-if="error"
			role="alert"
		>
			{{ error }} <button
				type="button"
				class="button"
				:disabled="disabled || busy"
				@click="load"
			>
				重新读取
			</button>
		</p>
	</section>
</template>

<script setup lang="ts">
import {ref, watch} from 'vue'
import {sharedOutstanding, readTaskHistory, changeOutstanding, type OutstandingItem} from '@/helpers/sharedOutstanding'
import ReadonlyRichText from './ReadonlyRichText.vue'
const props = defineProps<{taskId: number, disabled?: boolean}>()
const emit = defineEmits<{saved: []}>()
const items = ref<OutstandingItem[]>([])
const text = ref('')
const busy = ref(false)
const loading = ref(false)
const error = ref('')
async function load() {
	const id = props.taskId
	loading.value = true; error.value = ''
	try { const history = await readTaskHistory(id); if (id === props.taskId) items.value = sharedOutstanding(history).items }
	catch { if (id === props.taskId) error.value = '读取失败，请重试。' }
	finally { if (id === props.taskId) loading.value = false }
}
async function update(removeId?: string) {
	if (props.disabled || busy.value || loading.value || (!removeId && !text.value.trim())) return
	const taskId = props.taskId
	busy.value = true; error.value = ''
	const el = document.createElement('p'); el.textContent = text.value.trim()
	try {
		const result = await changeOutstanding(taskId, existing => removeId ? existing.filter(item => item.id !== removeId) : [...existing, {id: crypto.randomUUID(), html: el.innerHTML}])
		if (taskId !== props.taskId) return
		items.value = result; if (!removeId) text.value = ''; emit('saved')
	} catch { if (taskId === props.taskId) error.value = '保存失败，输入已保留，请重试。' }
	finally { busy.value = false }
}
const add = () => update()
const remove = (id: string) => update(id)
watch(() => props.taskId, () => { text.value = ''; void load() }, {immediate: true})
</script>
