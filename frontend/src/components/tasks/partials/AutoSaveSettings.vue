<template>
	<details class="auto-save-settings">
		<summary>自动保存设置</summary>
		<label><input
			v-model="autoSaveSettings.enabled"
			type="checkbox"
		> 启用自动保存</label>
		<label>检查间隔（秒）
			<input
				v-model.number="seconds"
				class="input"
				type="number"
				min="5"
				max="3600"
				step="1"
				@change="apply"
			>
		</label>
		<p>每 {{ autoSaveSettings.seconds }} 秒检查事项描述和当前进展，有变化才保存。进展会更新同一条记录，点击“保存进展”后结束本次编辑。设置自动保存在当前浏览器。</p>
		<p
			v-if="error"
			role="alert"
		>
			{{ error }}
		</p>
	</details>
</template>

<script setup lang="ts">
import {ref, watch} from 'vue'
import {autoSaveSettings} from '@/helpers/autoSave'
const seconds = ref(autoSaveSettings.seconds)
const error = ref('')
watch(() => autoSaveSettings.seconds, value => { seconds.value = value })
function apply() {
	if (!Number.isInteger(seconds.value) || seconds.value < 5 || seconds.value > 3600) { error.value = '请输入 5 到 3600 之间的整数秒。'; return }
	autoSaveSettings.seconds = seconds.value; error.value = ''
}
</script>

<style scoped lang="scss">
.auto-save-settings {
	margin-block: 1rem;
	summary {
		cursor: pointer;
		font-weight: 600;
	}
	label {
		display: block;
		margin-block: .75rem;
	}
	.input { max-inline-size: 8rem; }
}
</style>
