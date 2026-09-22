<template>
	<Modal
		variant="hint-modal"
		@close="$router.back()"
	>
		<Card
			class="has-no-shadow"
			:title="$t('about.title')"
			:padding="false"
			:show-close="true"
			@close="$router.back()"
		>
			<div class="p-4">
				<p v-if="versionsEqual">
					{{ $t('about.version', {version: apiVersion}) }}
				</p>
				<template v-else>
					<p>{{ $t('about.frontendVersion', {version: frontendVersion}) }}</p>
					<p>{{ $t('about.apiVersion', {version: apiVersion}) }}</p>
				</template>
			</div>
			<template #footer>
				<XButton
					v-if="isLocalBuild"
					variant="secondary"
					:loading="checkingChanges"
					@click.prevent.stop="openVersionChanges"
				>
					查看版本改动
				</XButton>
				<XButton
					variant="secondary"
					@click.prevent.stop="$router.back()"
				>
					{{ $t('misc.close') }}
				</XButton>
			</template>
		</Card>
	</Modal>
	<Modal
		:enabled="showVersionChanges"
		@close="showVersionChanges = false"
	>
		<div class="version-dialog">
			<h2>版本改动</h2>
			<VersionChanges
				:current-version="updateStore.state.current_version"
				:latest-version="updateStore.state.latest_version"
				:release-notes="updateStore.state.release_notes"
				:available="updateStore.available"
			/>
			<div class="version-actions">
				<XButton
					variant="tertiary"
					@click="showVersionChanges = false"
				>
					关闭
				</XButton>
				<XButton
					v-if="updateStore.available"
					@click="installUpdate"
				>
					更新到 {{ updateStore.state.latest_version }}
				</XButton>
			</div>
		</div>
	</Modal>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'

import {VERSION as frontendVersion} from '@/version.json'

import {useConfigStore} from '@/stores/config'
import {useTasktraceUpdateStore} from '@/stores/tasktraceUpdate'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {error as showError, success} from '@/message'
import VersionChanges from '@/components/update/VersionChanges.vue'

const configStore = useConfigStore()
const updateStore = useTasktraceUpdateStore()
const apiVersion = computed(() => configStore.version)
const versionsEqual = computed(() => apiVersion.value === frontendVersion)
const checkingChanges = ref(false)
const showVersionChanges = ref(false)

async function openVersionChanges() {
	checkingChanges.value = true
	try {
		await updateStore.checkNow()
		showVersionChanges.value = true
	} catch (cause) {
		showError(cause)
	} finally {
		checkingChanges.value = false
	}
}

async function installUpdate() {
	try {
		await updateStore.install()
		showVersionChanges.value = false
		success({message: '已开始更新，请在更新进度小窗中查看进度或取消。'})
	} catch (cause) {
		showError(cause)
	}
}
</script>

<style lang="scss" scoped>
.version-dialog {
	display: grid;
	gap: 1.25rem;
	box-sizing: border-box;
	inline-size: 100%;
	max-inline-size: 42rem;
	max-block-size: calc(100dvh - 2rem);
	padding: 1.5rem;
	overflow: auto;
	border-radius: $radius;
	background: var(--white);
	color: var(--text);
	box-shadow: var(--shadow-lg);
	text-align: start;

	h2 {
		margin: 0;
		color: var(--text-strong);
	}
}

.version-actions {
	display: flex;
	justify-content: flex-end;
	gap: .75rem;
}
</style>
