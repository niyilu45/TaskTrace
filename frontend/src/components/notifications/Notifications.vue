<template>
	<div class="notifications">
		<slot
			name="trigger"
			toggle-open="() => showNotifications = !showNotifications"
			:has-unread-notifications="unreadNotifications > 0"
		>
			<BaseButton
				class="trigger-button"
				:aria-expanded="showNotifications"
				@click.stop="showNotifications = !showNotifications"
			>
				<span class="is-sr-only">{{ $t('notification.title') }}</span>
				<span
					v-if="unreadNotifications > 0"
					class="unread-indicator"
				/>
				<Icon icon="bell" />
			</BaseButton>
		</slot>

		<CustomTransition name="fade">
			<div
				v-if="showNotifications"
				ref="popup"
				class="notifications-list"
			>
				<div class="head">
					<span>{{ $t('notification.title') }}</span>
					<div class="actions">
						<BaseButton
							v-if="notifications.length > 0 || teamStore.notificationCount > 0"
							v-tooltip="$t('notification.clearAll')"
							class="action-link"
							:aria-label="$t('notification.clearAll')"
							@click="clearAll"
						>
							<Icon icon="check-double" />
						</BaseButton>
						<BaseButton
							v-tooltip="$t('notification.subscribeFeed')"
							class="action-link"
							:to="{name: 'user.settings.feeds'}"
							@click="showNotifications = false"
						>
							<span class="is-sr-only">{{ $t('notification.subscribeFeed') }}</span>
							<Icon icon="rss" />
						</BaseButton>
					</div>
				</div>
				<button
					v-if="updateStore.shouldNotify"
					type="button"
					class="single-notification update-release-notification"
					@click="openUpdateDetails"
				>
					<span class="read-indicator" />
					<span class="detail">
						<strong>发现新版本 {{ updateStore.state.latest_version }}</strong>
						<span class="created">{{ displayReleaseDate(updateStore.state.published_at) }}</span>
					</span>
				</button>
				<button
					v-if="isLocalBuild && teamStore.conflictCount"
					type="button"
					class="single-notification team-notification-row"
					@click="openTeamActivity"
				>
					<span class="read-indicator" />
					<Icon icon="exclamation-circle" />
					<span class="detail">
						<strong>有 {{ teamStore.conflictCount }} 项团队协作冲突需要处理</strong>
						<span class="created">点击查看并一次性选择处理结果</span>
					</span>
				</button>
				<template v-if="isLocalBuild">
					<div
						v-for="notice in teamStore.status.notifications"
						:key="notice.id"
						class="single-notification team-notification-row"
					>
						<span class="read-indicator" />
						<img
							v-if="teamAvatar(notice.actor || '', notice.avatar)"
							:src="teamAvatar(notice.actor || '', notice.avatar)"
							alt=""
							class="team-notification-avatar"
						>
						<span
							v-else
							class="team-notification-avatar team-notification-avatar--fallback"
						>{{ initials(notice.actor || '') }}</span>
						<span class="detail">
							<span class="team-notification-message">
								<strong><TeamMemberIdentity :username="notice.actor || '协作成员'" /></strong> 更新了事项
								<button
									type="button"
									class="team-task-link"
									:title="`打开“${notice.task_title || '团队任务'}”的对应评论`"
									@click.stop="openTeamNotification(notice)"
								>
									“{{ notice.task_title || '团队任务' }}”
								</button>
							</span>
							<span class="created">{{ notice.created ? formatDisplayDate(notice.created) : '刚刚' }}</span>
						</span>
					</div>
				</template>
				<div
					v-for="(n, index) in notifications"
					:key="n.id"
					class="single-notification"
					:class="{'is-clickable': notificationHasRoute(n)}"
					@click="() => notificationHasRoute(n) && to(n, index)()"
				>
					<div
						class="read-indicator"
						:class="{'read': n.readAt !== null}"
					/>
					<User
						v-if="n.notification.doer"
						:user="n.notification.doer"
						:show-username="false"
						:avatar-size="16"
					/>
					<div class="detail">
						<div>
							<span
								v-if="n.notification.doer"
								class="has-text-weight-bold mie-1"
							>
								{{ getDisplayName(n.notification.doer) }}
							</span>
							{{ n.toText(userInfo) }}
						</div>
						<span
							v-tooltip="formatDateLong(n.created)"
							class="created"
						>
							{{ formatDisplayDate(n.created) }}
						</span>
					</div>
				</div>
				<XButton
					v-if="markableUnread > 0"
					variant="tertiary"
					class="mbs-2 is-fullwidth"
					@click="markAllRead"
				>
					{{ $t('notification.markAllRead') }}
				</XButton>
				<p
					v-if="notifications.length === 0 && !updateStore.shouldNotify && teamStore.activityCount === 0"
					class="nothing"
				>
					{{ $t('notification.none') }}<br>
					<span class="explainer">
						{{ $t('notification.explainer') }}
					</span>
				</p>
			</div>
		</CustomTransition>
		<Modal
			:enabled="showUpdateDetails"
			@close="declineUpdate"
			@submit="installUpdate"
		>
			<template #header>
				发现新版本 {{ updateStore.state.latest_version }}
			</template>
			<template #text>
				<p><strong>发布日期：</strong>{{ displayReleaseDate(updateStore.state.published_at) }}</p>
				<p class="mbs-3">
					<strong>更新内容：</strong>
				</p>
				<pre class="release-notes">{{ updateStore.state.release_notes || '本次发布未填写更新内容。' }}</pre>
				<p class="mbs-4">
					更新需要关闭正在运行的 TaskTrace。确认后将使用 Windows 系统代理下载更新，随后关闭、替换文件并自动重新启动。
				</p>
			</template>
		</Modal>
	</div>
</template>

<script lang="ts" setup>
import {computed, onMounted, onUnmounted, ref, watch} from 'vue'
import {useRouter, isNavigationFailure, NavigationFailureType, type RouteLocationRaw} from 'vue-router'

import NotificationService from '@/services/notification'
import NotificationModel from '@/models/notification'
import BaseButton from '@/components/base/BaseButton.vue'
import CustomTransition from '@/components/misc/CustomTransition.vue'
import User from '@/components/misc/User.vue'
import {NOTIFICATION_NAMES as names, type INotification} from '@/modelTypes/INotification'
import {closeWhenClickedOutside} from '@/helpers/closeWhenClickedOutside'
import {formatDateLong, formatDisplayDate} from '@/helpers/time/formatDate'
import {getDisplayName} from '@/models/user'
import {useAuthStore} from '@/stores/auth'
import {useWebSocket} from '@/composables/useWebSocket'
import XButton from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import {error as showError, success} from '@/message'
import {useI18n} from 'vue-i18n'
import {useTasktraceUpdateStore} from '@/stores/tasktraceUpdate'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import {scrollAndHighlightComment} from '@/components/tasks/partials/commentReplyContext'
import TeamMemberIdentity from '@/components/tasks/partials/TeamMemberIdentity.vue'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import type {TaskTraceTeamNotification} from '@/client/generated'

const {subscribe, connected: wsConnected} = useWebSocket()

const authStore = useAuthStore()
const router = useRouter()
const {t} = useI18n()
const updateStore = useTasktraceUpdateStore()
const teamStore = useTasktraceTeamStore()

const allNotifications = ref<INotification[]>([])
const showNotifications = ref(false)
const showUpdateDetails = ref(false)
const popup = ref(null)

const unreadNotifications = computed(() => {
	return notifications.value.filter(n => n.readAt === null).length + (updateStore.shouldNotify ? 1 : 0) + (isLocalBuild ? teamStore.activityCount : 0)
})
const markableUnread = computed(() => notifications.value.filter(n => n.readAt === null).length + (isLocalBuild ? teamStore.notificationCount : 0))
const notifications = computed(() => {
	return allNotifications.value ? allNotifications.value.filter(n => n.name !== '') : []
})
const userInfo = computed(() => authStore.info)

let unsubscribeWs: (() => void) | null = null
let pollInterval: ReturnType<typeof setInterval> | null = null
let updatePollInterval: ReturnType<typeof setInterval> | null = null

const POLL_INTERVAL = 10000

onMounted(async () => {
	// Initial load via REST - wrapped in try/catch so the rest of setup
	// (click handler, WS subscription, polling) still runs if this fails
	try {
		await loadNotifications()
	} catch (e) {
		console.warn('Failed to load initial notifications:', e)
	}

	document.addEventListener('click', hidePopup)

	// Subscribe to real-time notifications
	unsubscribeWs = subscribe('notification.created', (msg) => {
		if (msg.event === 'notification.created' && msg.data) {
			const notification = new NotificationModel(msg.data as Partial<INotification>)
			// Avoid duplicates if the same notification was already loaded via REST
			const exists = allNotifications.value.some(n => n.id === notification.id)
			if (!exists) {
				allNotifications.value = [notification, ...allNotifications.value]
			}
		}
	})

	// Fallback polling when WebSocket is not available
	startPollingFallback()
	try { await updateStore.refresh() } catch { /* Updates are available only in the local portable build. */ }
	updatePollInterval = setInterval(() => updateStore.refresh().catch(() => undefined), 15_000)
})

// Reload notifications when WebSocket disconnects to catch any events
// that may have been missed during the disconnect window
watch(wsConnected, (isConnected, wasConnected) => {
	if (wasConnected && !isConnected) {
		loadNotifications().catch(e => console.warn('Failed to reload notifications after WS disconnect:', e))
	}
})

onUnmounted(() => {
	document.removeEventListener('click', hidePopup)
	unsubscribeWs?.()
	stopPollingFallback()
	if (updatePollInterval) clearInterval(updatePollInterval)
})

function openUpdateDetails() {
	showNotifications.value = false
	showUpdateDetails.value = true
}

function openTeamActivity() {
	showNotifications.value = false
	window.dispatchEvent(new CustomEvent('tasktrace-team-activity-open'))
}

async function waitForTeamComment(commentId: number) {
	const deadline = Date.now() + 4000
	do {
		if (document.getElementById(`comment-${commentId}`)) {
			scrollAndHighlightComment(commentId)
			return true
		}
		await new Promise(resolve => window.setTimeout(resolve, 50))
	} while (Date.now() < deadline)
	return false
}

async function openTeamNotification(notice: TaskTraceTeamNotification) {
	let target = notice
	if ((!target.task_id || (!target.comment_id && target.shared_comment_id)) && target.id) {
		try {
			await teamStore.sync()
			target = teamStore.status.notifications?.find(candidate => candidate.id === target.id) || target
		} catch (cause) {
			console.warn('Failed to synchronize the team notification target:', cause)
		}
	}
	const taskId = Number(target.task_id || 0)
	if (!taskId) {
		openTeamActivity()
		return
	}
	showNotifications.value = false
	const commentId = Number(target.comment_id || 0)
	const route: RouteLocationRaw = {
		name: 'task.detail',
		params: {id: taskId},
		...(commentId ? {hash: `#comment-${commentId}`} : {}),
	}
	const failure = await router.push(route)
	if (commentId) {
		if (isNavigationFailure(failure, NavigationFailureType.duplicated)) {
			scrollAndHighlightComment(commentId)
		}
		await waitForTeamComment(commentId)
	}
	if (notice.id) {
		try {
			await teamStore.dismissNotifications([notice.id])
		} catch (cause) {
			console.warn('Failed to mark team notification as read:', cause)
		}
	}
}

function teamAvatar(username: string, preferred = '') {
	return preferred || teamStore.avatarFor(username)
}

function initials(username: string) {
	return username.trim().slice(0, 2).toUpperCase() || '?'
}

async function declineUpdate() {
	showUpdateDetails.value = false
	try { await updateStore.ignore() } catch (cause) { showError(cause) }
}

async function installUpdate() {
	try {
		await updateStore.install()
		showUpdateDetails.value = false
		success({message: '正在下载更新，完成后 TaskTrace 将关闭并重新启动。'})
	} catch (cause) {
		showError(cause)
	}
}

function displayReleaseDate(value?: string) {
	if (!value) return '发布日期未知'
	const date = new Date(value)
	return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function startPollingFallback() {
	pollInterval = setInterval(async () => {
		if (!wsConnected.value && document.visibilityState === 'visible') {
			await loadNotifications()
		}
	}, POLL_INTERVAL)
}

function stopPollingFallback() {
	if (pollInterval) {
		clearInterval(pollInterval)
		pollInterval = null
	}
}

async function loadNotifications() {
	const notificationService = new NotificationService()
	allNotifications.value = await notificationService.getAll()
}

function hidePopup(e) {
	if (showNotifications.value) {
		closeWhenClickedOutside(e, popup.value, () => showNotifications.value = false)
	}
}

function getNotificationRoute(n: INotification): RouteLocationRaw | null {
	switch (n.name) {
		case names.TASK_COMMENT:
		case names.TASK_ASSIGNED:
		case names.TASK_REMINDER:
		case names.TASK_MENTIONED:
		case names.TASK_CREATED:
			return {name: 'task.detail', params: {id: (n.notification as {task: {id: number}}).task.id}}
		case names.PROJECT_CREATED:
			return {name: 'task.index', params: {projectId: (n.notification as {project: {id: number}}).project.id}}
		case names.TEAM_MEMBER_ADDED:
			return {name: 'teams.edit', params: {id: (n.notification as {team: {id: number}}).team.id}}
		default:
			return null
	}
}

function notificationHasRoute(n: INotification): boolean {
	return getNotificationRoute(n) !== null
}

function to(n: INotification, index: number) {
	return async () => {
		const route = getNotificationRoute(n)
		if (route === null) return
		
		const failure = await router.push(route)
		if (isNavigationFailure(failure, NavigationFailureType.duplicated)) {
			router.go(0)
		}

		n.read = true
		if (allNotifications.value[index]) {
			const notificationService = new NotificationService()
			Object.assign(allNotifications.value[index], await notificationService.update(n))
		}

		showNotifications.value = false
	}
}

async function markAllRead() {
	const notificationService = new NotificationService()
	await Promise.all([
		notifications.value.some(n => n.readAt === null) ? notificationService.markAllRead() : Promise.resolve(),
		isLocalBuild && teamStore.notificationCount ? teamStore.dismissNotifications() : Promise.resolve(),
	])
	success({message: t('notification.markAllReadSuccess')})

	notifications.value.forEach(n => n.readAt = new Date())
}

async function clearAll() {
	const notificationService = new NotificationService()
	await Promise.all([
		notifications.value.length ? notificationService.delete(new NotificationModel({})) : Promise.resolve(),
		isLocalBuild && teamStore.notificationCount ? teamStore.dismissNotifications() : Promise.resolve(),
	])
	success({message: t('notification.clearAllSuccess')})
	allNotifications.value = []
}
</script>

<style lang="scss" scoped>
.notifications {
	display: flex;

	.trigger-button {
		inline-size: 100%;
		position: relative;
	}

	.unread-indicator {
		position: absolute;
		inset-block-start: 1rem;
		inset-inline-end: .5rem;
		inline-size: .75rem;
		block-size: .75rem;

		background: var(--primary);
		border-radius: 100%;
		border: 2px solid var(--white);
	}

	.notifications-list {
		position: absolute;
		inset-inline-end: 1rem;
		inset-block-start: calc(100% + 1rem);
		max-block-size: 400px;
		overflow-y: auto;

		background: var(--white);
		color: var(--text);
		inline-size: 350px;
		max-inline-size: calc(100vw - 2rem);
		padding: .75rem .25rem;
		border-radius: $radius;
		box-shadow: var(--shadow-sm);
		font-size: .85rem;

		@media screen and (max-width: $tablet) {
			max-block-size: calc(100vh - 1rem - #{$navbar-height});
		}

		.head {
			font-family: $vikunja-font;
			font-size: 1rem;
			padding: .5rem;
			display: flex;
			align-items: center;
			justify-content: space-between;

			.actions {
				display: flex;
				align-items: center;
				gap: .5rem;
			}

			.action-link {
				color: var(--grey-500);
				transition: color $transition;

				&:hover,
				&:focus {
					color: var(--primary);
				}
			}
		}

		.single-notification {
			display: flex;
			align-items: center;
			padding: 0.25rem 0;

			transition: background-color $transition;

			&.is-clickable {
				cursor: pointer;
			}

			&:hover {
				background: var(--grey-100);
				border-radius: $radius;
			}

			.read-indicator {
				inline-size: .35rem;
				block-size: .35rem;
				background: var(--primary);
				border-radius: 100%;
				margin: 0 .5rem;
				flex-shrink: 0;

				&.read {
					background: transparent;
				}
			}

			.user {
				display: inline-flex;
				align-items: center;
				inline-size: auto;
				margin: 0 .5rem;

				span {
					font-family: $family-sans-serif;
				}

				.avatar {
					block-size: 16px;
				}

				img {
					margin-inline-end: 0;
				}
			}

			.created {
				color: var(--grey-400);
			}

			&:last-child {
				margin-block-end: .25rem;
			}

			a {
				color: var(--grey-800);
			}
		}

		.nothing {
			text-align: center;
			padding: 1rem 0;
			color: var(--grey-500);

			.explainer {
				font-size: .75rem;
			}
		}
	}

	.update-release-notification {
		inline-size: 100%;
		border: 0;
		background: transparent;
		font: inherit;
		text-align: start;
		color: var(--grey-800);
		cursor: pointer;

		.detail {
			display: grid;
			gap: .125rem;
		}
	}

	.team-notification-row {
		inline-size: 100%;
		gap: .45rem;
		border: 0;
		background: transparent;
		color: var(--grey-800);
		font: inherit;
		text-align: start;
		cursor: default;
		.detail {
			display: grid;
			min-inline-size: 0;
			gap: .125rem;
		}
		strong, .detail > span:first-child { overflow-wrap: anywhere; }
	}

	.team-notification-message {
		color: var(--text);

		strong {
			color: var(--text-strong);
		}
	}

	.team-task-link {
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--primary);
		font: inherit;
		font-weight: 700;
		text-align: start;
		text-decoration: underline;
		text-underline-offset: 2px;
		cursor: pointer;

		&:hover,
		&:focus-visible {
			color: var(--link-hover);
		}
	}

	.team-notification-avatar {
		inline-size: 1.5rem;
		block-size: 1.5rem;
		flex: 0 0 1.5rem;
		border-radius: 50%;
		object-fit: cover;
	}

	.team-notification-avatar--fallback {
		display: inline-grid;
		place-items: center;
		background: var(--primary);
		color: var(--white);
		font-size: .6rem;
		font-weight: 700;
	}

}

.release-notes {
	max-block-size: 16rem;
	overflow: auto;
	white-space: pre-wrap;
	font: inherit;
	background: var(--grey-100);
	border-radius: $radius;
	padding: .75rem;
}
</style>
