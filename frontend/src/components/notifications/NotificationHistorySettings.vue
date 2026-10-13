<template>
	<Card
		v-if="isLocalBuild"
		title="通知历史"
		class="section-block"
	>
		<p class="help">
			清除通知后会移入历史通知。超过保留数量时，自动清理最早收纳的记录。
		</p>
		<FormField
			label="最多保留的历史通知数量"
			layout="two-col"
		>
			<FormInput
				v-model.number="limit"
				type="number"
				min="1"
				max="10000"
				step="1"
				:disabled="loading || saving"
			/>
		</FormField>
		<p
			v-if="loadError"
			class="notification is-danger is-light"
			role="alert"
		>
			{{ loadError }}
		</p>
		<XButton
			v-if="loadError"
			variant="secondary"
			:loading="loading"
			@click="load"
		>
			重新读取设置
		</XButton>
		<XButton
			v-else
			:loading="saving"
			:disabled="loading || saving"
			@click="save"
		>
			保存通知设置
		</XButton>
	</Card>
</template>

<script setup lang="ts">
import {onMounted, ref} from 'vue'
import {tasktraceNotificationSettingsRead, tasktraceNotificationSettingsWrite} from '@/client/generated'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {error, getErrorText, success} from '@/message'
import Card from '@/components/misc/Card.vue'
import FormField from '@/components/input/FormField.vue'
import FormInput from '@/components/input/FormInput.vue'
import XButton from '@/components/input/Button.vue'

const limit = ref<number | string>(100)
const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
async function load() {
	if (!isLocalBuild || loading.value) return
	loading.value = true
	loadError.value = ''
	try { limit.value = (await tasktraceNotificationSettingsRead()).data.max_entries ?? 100 }
	catch (cause) { loadError.value = `无法读取通知设置：${getErrorText(cause)}` }
	finally { loading.value = false }
}
async function save() {
	const value = Number(limit.value)
	if (!Number.isSafeInteger(value) || value < 1 || value > 10000) {
		error({message: '历史通知保留数量需要填写 1 到 10000 之间的整数。'})
		return
	}
	if (saving.value || loading.value || loadError.value) return
	saving.value = true
	try {
		limit.value = (await tasktraceNotificationSettingsWrite({body: {max_entries: value}})).data.max_entries ?? value
		success({message: '通知设置已保存，超出数量的历史通知已清理。'})
	} catch (cause) { error(cause) }
	finally { saving.value = false }
}
onMounted(load)
</script>
