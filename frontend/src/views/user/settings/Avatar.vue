<template>
	<Card :title="$t('user.settings.avatar.title')">
		<div class="avatar-settings-layout">
			<aside
				class="avatar-preview-panel"
				:aria-busy="avatarService.loading || loading || providerSaving"
				aria-live="polite"
			>
				<h3>{{ $t('user.settings.avatar.livePreview') }}</h3>
				<div
					class="avatar-preview-frame"
					:class="{'is-updating': avatarService.loading || loading || providerSaving}"
				>
					<canvas
						v-if="isCropAvatar"
						ref="previewCanvas"
						width="144"
						height="144"
						class="avatar-preview-image"
						:aria-label="$t('user.settings.avatar.cropPreviewAlt')"
						role="img"
					/>
					<UserAvatar
						v-else
						:user="authStore.info"
						:size="144"
						:alt="$t('user.settings.avatar.currentPreviewAlt')"
						class="avatar-preview-image"
					/>
				</div>
				<p>{{ previewDescription }}</p>
			</aside>

			<div class="avatar-settings-controls">
				<Message v-if="avatarProvider === 'ldap'">
					{{ $t('user.settings.avatar.ldap') }}
				</Message>

				<Message v-else-if="avatarProvider === 'openid'">
					{{ $t('user.settings.avatar.openid', {provider: authProviderName}) }}
				</Message>

				<template v-else>
					<fieldset
						class="avatar-provider-options mbe-4"
						:disabled="providerSaving"
					>
						<legend>{{ $t('user.settings.avatar.chooseProvider') }}</legend>
						<label
							v-for="(label, providerId) in AVATAR_PROVIDERS"
							:key="providerId"
							class="radio"
						>
							<input
								v-model="avatarProvider"
								name="avatarProvider"
								type="radio"
								:value="providerId"
								@change="changeAvatarProvider"
							>
							{{ label }}
						</label>
					</fieldset>

					<template v-if="avatarProvider === 'upload'">
						<input
							ref="avatarUploadInput"
							accept="image/*"
							class="is-hidden"
							type="file"
							@change="cropAvatar"
						>

						<XButton
							v-if="!isCropAvatar"
							:loading="avatarService.loading || loading"
							@click="openAvatarPicker"
						>
							{{ $t('user.settings.avatar.chooseImage') }}
						</XButton>
						<template v-else>
							<Cropper
								ref="cropper"
								:src="avatarToCrop"
								:stencil-props="{aspectRatio: 1}"
								class="mbe-4 cropper"
								@change="updateCropPreview"
								@ready="onCropperReady"
							/>
							<div class="avatar-actions">
								<XButton
									v-cy="'uploadAvatar'"
									:loading="avatarService.loading || loading"
									@click="uploadAvatar"
								>
									{{ $t('user.settings.avatar.uploadAvatar') }}
								</XButton>
								<XButton
									variant="secondary"
									:disabled="avatarService.loading || loading"
									@click="cancelCrop"
								>
									{{ $t('misc.cancel') }}
								</XButton>
							</div>
						</template>
					</template>

					<p
						v-else
						class="avatar-save-hint"
					>
						{{ $t('user.settings.avatar.autoSaveHint') }}
					</p>
				</template>
			</div>
		</div>
	</Card>
</template>


<script setup lang="ts">
import {computed, nextTick, ref, shallowReactive} from 'vue'
import {useI18n} from 'vue-i18n'
import {Cropper, type CropperResult} from 'vue-advanced-cropper'
import 'vue-advanced-cropper/dist/style.css'

import AvatarService from '@/services/avatar'
import AvatarModel from '@/models/avatar'
import type {AvatarProvider} from '@/modelTypes/IAvatar'
import {useTitle} from '@/composables/useTitle'
import {error, success} from '@/message'
import {useAuthStore} from '@/stores/auth'
import Message from '@/components/misc/Message.vue'
import UserAvatar from '@/components/misc/UserAvatar.vue'

defineOptions({name: 'UserSettingsAvatar'})

type ManagedAvatarProvider = 'ldap' | 'openid'
type AvailableAvatarProvider = AvatarProvider | ManagedAvatarProvider | ''

const {t} = useI18n({useScope: 'global'})
const authStore = useAuthStore()
const authProviderName = computed(() => (authStore.info as unknown as ({authProvider?: string} | null))?.authProvider ?? '')

const AVATAR_PROVIDERS = computed<Record<AvatarProvider, string>>(() => ({
	default: t('misc.default'),
	initials: t('user.settings.avatar.initials'),
	gravatar: t('user.settings.avatar.gravatar'),
	marble: t('user.settings.avatar.marble'),
	upload: t('user.settings.avatar.upload'),
}))

useTitle(() => `${t('user.settings.avatar.title')} - ${t('user.settings.title')}`)

const avatarService = shallowReactive(new AvatarService())
const loading = ref(false)
const providerSaving = ref(false)
const avatarProvider = ref<AvailableAvatarProvider>('')
const savedAvatarProvider = ref<AvailableAvatarProvider>('')
const cropper = ref<InstanceType<typeof Cropper> | null>(null)
const isCropAvatar = ref(false)
const avatarToCrop = ref('')
const avatarUploadInput = ref<HTMLInputElement | null>(null)
const previewCanvas = ref<HTMLCanvasElement | null>(null)

const previewDescription = computed(() => {
	if (providerSaving.value) return t('user.settings.avatar.previewUpdating')
	if (isCropAvatar.value) return t('user.settings.avatar.cropPreviewHint')
	return t('user.settings.avatar.previewHint')
})

function isSelectableProvider(provider: AvailableAvatarProvider): provider is AvatarProvider {
	return Object.prototype.hasOwnProperty.call(AVATAR_PROVIDERS.value, provider)
}

async function avatarStatus() {
	try {
		const {avatarProvider: currentProvider} = await avatarService.get(new AvatarModel({}))
		avatarProvider.value = currentProvider
		savedAvatarProvider.value = currentProvider
	} catch (reason) {
		error(reason)
	}
}

void avatarStatus()

async function changeAvatarProvider() {
	const nextProvider = avatarProvider.value
	if (!isSelectableProvider(nextProvider) || nextProvider === savedAvatarProvider.value || providerSaving.value) return

	providerSaving.value = true
	try {
		await avatarService.update(new AvatarModel({avatarProvider: nextProvider}))
		avatarProvider.value = nextProvider
		savedAvatarProvider.value = nextProvider
		if (nextProvider !== 'upload') resetCrop()
		authStore.invalidateAvatar()
		success({message: t('user.settings.avatar.statusUpdateSuccess')})
	} catch (reason) {
		avatarProvider.value = savedAvatarProvider.value
		error(reason)
	} finally {
		providerSaving.value = false
	}
}

function drawCropPreview(result?: CropperResult) {
	const source = result?.canvas ?? cropper.value?.getResult().canvas
	const target = previewCanvas.value
	if (!source || !target) return

	const context = target.getContext('2d')
	if (!context) return
	context.clearRect(0, 0, target.width, target.height)
	context.imageSmoothingEnabled = true
	context.imageSmoothingQuality = 'high'
	context.drawImage(source, 0, 0, target.width, target.height)
}

function updateCropPreview(result: CropperResult) {
	drawCropPreview(result)
}

async function onCropperReady() {
	loading.value = false
	await nextTick()
	drawCropPreview()
}

function resetCrop() {
	isCropAvatar.value = false
	avatarToCrop.value = ''
	if (avatarUploadInput.value) avatarUploadInput.value.value = ''
}

function cancelCrop() {
	resetCrop()
}

function openAvatarPicker() {
	if (!avatarUploadInput.value) return
	avatarUploadInput.value.value = ''
	avatarUploadInput.value.click()
}

async function uploadAvatar() {
	const canvas = cropper.value?.getResult().canvas
	if (!canvas) return

	loading.value = true
	try {
		const blob = await new Promise<Blob>((resolve, reject) => {
			canvas.toBlob(value => value ? resolve(value) : reject(new Error(t('user.settings.avatar.previewUnavailable'))), 'image/png')
		})
		await avatarService.create(blob)
		avatarProvider.value = 'upload'
		savedAvatarProvider.value = 'upload'
		authStore.invalidateAvatar()
		resetCrop()
		success({message: t('user.settings.avatar.setSuccess')})
	} catch (reason) {
		error(reason)
	} finally {
		loading.value = false
	}
}

function cropAvatar() {
	const file = avatarUploadInput.value?.files?.[0]
	if (!file) return

	loading.value = true
	const reader = new FileReader()
	reader.onload = event => {
		if (typeof event.target?.result !== 'string') {
			loading.value = false
			error(new Error(t('user.settings.avatar.previewUnavailable')))
			return
		}
		avatarToCrop.value = event.target.result
		isCropAvatar.value = true
	}
	reader.onerror = () => {
		loading.value = false
		error(new Error(t('user.settings.avatar.previewUnavailable')))
	}
	reader.readAsDataURL(file)
}
</script>

<style lang="scss">
.cropper {
	block-size: min(62vh, 36rem);
	min-block-size: 20rem;
	background: transparent;
}

.vue-advanced-cropper__background {
	background: var(--white);
}
</style>

<style lang="scss" scoped>
.avatar-settings-layout {
	display: grid;
	grid-template-columns: minmax(11rem, 14rem) minmax(0, 1fr);
	gap: 2rem;
	align-items: start;
}

.avatar-preview-panel {
	position: sticky;
	inset-block-start: 1rem;
	text-align: center;

	h3 {
		font-size: 1rem;
		font-weight: 600;
		margin-block-end: 1rem;
	}

	p {
		color: var(--grey-600);
		font-size: .875rem;
		line-height: 1.5;
		margin-block-start: .75rem;
	}
}

.avatar-preview-frame {
	display: grid;
	place-items: center;
	inline-size: 9rem;
	block-size: 9rem;
	margin-inline: auto;
	border: 1px solid var(--grey-300);
	border-radius: 50%;
	background: var(--grey-100);
	overflow: hidden;
	transition: opacity $transition;

	&.is-updating {
		opacity: .58;
	}
}

.avatar-preview-image {
	display: block;
	inline-size: 100%;
	block-size: 100%;
	object-fit: cover;
}

.avatar-settings-controls {
	min-inline-size: 0;
}

.avatar-provider-options {
	border: 0;
	padding: 0;

	legend {
		font-weight: 600;
		margin-block-end: .75rem;
	}
}

.avatar-actions {
	display: flex;
	flex-wrap: wrap;
	gap: .75rem;
}

.avatar-save-hint {
	color: var(--grey-600);
	font-size: .875rem;
	margin: 0;
}

// Ported from bulma-css-variables/sass/form/checkbox-radio.sass
// (the %checkbox-radio placeholder), scoped to this component so we can
// drop the global Bulma import.
label.radio {
	cursor: pointer;
	display: inline-block;
	line-height: 1.25;
	position: relative;

	input {
		cursor: pointer;
	}

	&:hover {
		color: var(--input-hover-color);
	}

	&[disabled],
	input[disabled] {
		color: var(--input-disabled-color);
		cursor: not-allowed;
	}

	& + .radio {
		margin-inline-start: .5em;
	}
}

@media (width <= 48rem) {
	.avatar-settings-layout {
		grid-template-columns: 1fr;
		gap: 1.5rem;
	}

	.avatar-preview-panel {
		position: static;
	}
}
</style>
