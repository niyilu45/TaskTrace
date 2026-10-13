<template>
	<div class="notifications">
		<slot
			name="trigger"
			:toggle-open="() => showNotifications = !showNotifications"
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
					<span>{{ showHistory ? '历史通知' : $t('notification.title') }}</span>
					<div class="actions">
						<BaseButton
							v-if="isLocalBuild"
							class="history-toggle action-link"
							:aria-pressed="showHistory"
							@click="showHistory = !showHistory"
						>
							{{ showHistory ? '当前通知' : '历史通知' }}
						</BaseButton>
						<BaseButton
							v-if="!showHistory && (notifications.length > 0 || teamStore.notificationCount > 0)"
							v-tooltip="$t('notification.clearAll')"
							class="action-link"
							:aria-label="$t('notification.clearAll')"
							:disabled="clearingAll"
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
				<NotificationHistory
					v-if="showHistory"
					@openTeam="notice => openTeamNotification(notice, true)"
					@openNotification="notice => to(notice)()"
				/>
				<template v-else>
					<div
						class="reading-status"
						role="status"
					>
						<button
							v-if="hasPendingChanges"
							type="button"
							@click="applyPendingNotifications"
						>
							通知有更新，点击查看
						</button>
						<span v-else>阅读期间列表保持不动</span>
					</div>
					<button
						v-if="readingUpdate"
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
						v-if="isLocalBuild && readingConflictCount"
						type="button"
						class="single-notification team-notification-row"
						@click="openTeamConflicts"
					>
						<span class="read-indicator" />
						<Icon icon="exclamation-circle" />
						<span class="detail">
							<strong>有 {{ readingConflictCount }} 项团队协作冲突需要处理</strong>
							<span class="created">点击对比差异，逐项处理</span>
						</span>
					</button>
					<template v-if="isLocalBuild">
						<div
							v-for="notice in readingTeamNotifications"
							:key="notice.id"
							class="single-notification team-notification-row"
						>
							<span
								class="read-indicator"
								:class="{read: notice.read}"
								:aria-label="notice.read ? '已读' : '未读'"
							/>
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
										:disabled="openingTeamNotification"
										:aria-busy="openingTeamNotification"
										@click.stop="openTeamNotification(notice)"
									>
										“{{ notice.task_title || '团队任务' }}”
									</button>
								</span>
								<span class="created">{{ notice.created ? formatDisplayDate(notice.created) : '刚刚' }}</span>
							</span>
							<button
								type="button"
								class="notification-delete"
								aria-label="清除此条团队通知（移入历史）"
								:disabled="deleting.has(`team:${notice.id}`)"
								@click.stop="deleteTeamNotification(notice)"
							>
								<Icon icon="trash-alt" />
							</button>
						</div>
					</template>
					<div
						v-for="n in notifications"
						:key="n.id"
						class="single-notification"
						:class="{'is-clickable': notificationHasRoute(n)}"
						@click="() => notificationHasRoute(n) && to(n)()"
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
						<button
							type="button"
							class="notification-delete"
							aria-label="清除此条通知（移入历史）"
							:disabled="deleting.has(`notice:${n.id}`)"
							@click.stop="deleteNotification(n)"
						>
							<Icon icon="trash-alt" />
						</button>
					</div>
					<XButton
						v-if="loadedPages < totalPages"
						variant="tertiary"
						class="is-fullwidth"
						:loading="loadingMore"
						:disabled="loadingMore"
						@click="loadMoreNotifications"
					>
						加载更早的通知
					</XButton>
					<XButton
						v-if="markableUnread > 0"
						variant="tertiary"
						class="mbs-2 is-fullwidth"
						@click="markAllRead"
					>
						{{ $t('notification.markAllRead') }}
					</XButton>
					<p
						v-if="notifications.length === 0 && !readingUpdate && readingTeamNotifications.length === 0 && readingConflictCount === 0"
						class="nothing"
					>
						{{ $t('notification.none') }}<br>
						<span class="explainer">
							{{ $t('notification.explainer') }}
						</span>
					</p>
				</template>
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
				<VersionChanges
					class="mbs-3"
					:current-version="updateStore.state.current_version"
					:latest-version="updateStore.state.latest_version"
					:release-notes="updateStore.state.release_notes"
					:available="updateStore.available"
				/>
				<p class="mbs-4">
					确认更新后会打开进度小窗。下载或替换期间可以取消，原版本会保持不变或自动恢复。
				</p>
			</template>
		</Modal>
	</div>
</template>

<script lang="ts" setup>
import {computed, onMounted, onUnmounted, ref, watch, nextTick} from 'vue'
import equal from 'fast-deep-equal'
import {useVisiblePolling} from '@/composables/useVisiblePolling'
import {useRouter, isNavigationFailure, NavigationFailureType, type RouteLocationRaw} from 'vue-router'

import NotificationHistory from './NotificationHistory.vue'
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
import VersionChanges from '@/components/update/VersionChanges.vue'
import {error as showError, success} from '@/message'
import {useI18n} from 'vue-i18n'
import {useTasktraceUpdateStore} from '@/stores/tasktraceUpdate'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import {scrollAndHighlightComment} from '@/components/tasks/partials/commentReplyContext'
import TeamMemberIdentity from '@/components/tasks/partials/TeamMemberIdentity.vue'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {notificationsDelete, tasktraceNotificationHistoryList, type TaskTraceTeamNotification} from '@/client/generated'

const {subscribe, connected: wsConnected} = useWebSocket()

const authStore = useAuthStore()
const router = useRouter()
const {t} = useI18n()
const updateStore = useTasktraceUpdateStore()
const teamStore = useTasktraceTeamStore()

const allNotifications = ref<INotification[]>([])
const showNotifications = ref(false)
const showHistory = ref(false)
const clearingAll = ref(false)
const showUpdateDetails = ref(false)
const popup = ref<HTMLElement | null>(null)
const readingNotifications = ref<INotification[]>([])
const readingTeamNotifications = ref<TaskTraceTeamNotification[]>([])
const readingConflictCount = ref(0)
const readingUpdate = ref(false)
const loadedPages = ref(1)
const totalPages = ref(1)
const loadingMore = ref(false)
const deleting = ref(new Set<string>())
const openingTeamNotification = ref(false)
const deletedNotificationIds = new Set<number>()
let initialReadFinished = false

function captureNotifications() {
	readingNotifications.value = [...allNotifications.value]
	readingTeamNotifications.value = [...(teamStore.status.notifications ?? [])]
	readingConflictCount.value = teamStore.conflictCount || 0
	readingUpdate.value = updateStore.shouldNotify
}
watch(showNotifications, open => { if (open) captureNotifications() })
const hasPendingChanges = computed(() => !equal(readingNotifications.value, allNotifications.value)
	|| !equal(readingTeamNotifications.value, teamStore.status.notifications ?? [])
	|| readingConflictCount.value !== (teamStore.conflictCount || 0)
	|| readingUpdate.value !== updateStore.shouldNotify)

async function applyPendingNotifications() {
	captureNotifications()
	await nextTick()
	if (popup.value) popup.value.scrollTop = 0
}

async function deleteNotification(notice: INotification) {
	const key = `notice:${notice.id}`
	if (deleting.value.has(key)) return
	deleting.value.add(key)
	notificationsRevision++
	try {
		await notificationsDelete({path: {notificationid: notice.id}})
		deletedNotificationIds.add(notice.id)
		notificationsRevision++
		allNotifications.value = allNotifications.value.filter(item => item.id !== notice.id)
		readingNotifications.value = readingNotifications.value.filter(item => item.id !== notice.id)
	} catch (cause) { showError(cause) }
	finally { deleting.value.delete(key) }
}

async function deleteTeamNotification(notice: TaskTraceTeamNotification) {
	if (!notice.id) return
	const key = `team:${notice.id}`
	if (deleting.value.has(key)) return
	deleting.value.add(key)
	try {
		await teamStore.dismissNotifications([notice.id])
		readingTeamNotifications.value = readingTeamNotifications.value.filter(item => item.id !== notice.id)
	} catch (cause) { showError(cause) }
	finally { deleting.value.delete(key) }
}

async function loadMoreNotifications() {
	if (loadingMore.value) return
	loadingMore.value = true
	const previousPages = loadedPages.value
	loadedPages.value++
	notificationsRevision++
	try {
		await loadNotifications()
		const known = new Set(readingNotifications.value.map(item => item.id))
		readingNotifications.value.push(...allNotifications.value.filter(item => !known.has(item.id)))
	} catch (cause) { loadedPages.value = previousPages; showError(cause) }
	finally { loadingMore.value = false }
}

const unreadNotifications = computed(() => {
	return allNotifications.value.filter(n => n.name !== '' && n.readAt === null).length + (updateStore.shouldNotify ? 1 : 0) + (isLocalBuild ? teamStore.activityCount : 0)
})
const markableUnread = computed(() => notifications.value.filter(n => n.readAt === null).length + (isLocalBuild ? teamStore.unreadNotificationCount : 0))
const notifications = computed(() => {
	return (showNotifications.value ? readingNotifications.value : allNotifications.value).filter(n => n.name !== '')
})
const userInfo = computed(() => authStore.info)

let unsubscribeWs: (() => void) | null = null
let activeRead: Promise<void> | undefined
let notificationsRevision = 0
let disposed = false

const POLL_INTERVAL = 10000
const {refresh: refreshNotifications} = useVisiblePolling(loadNotifications, POLL_INTERVAL, {immediate: false, enabled: () => !wsConnected.value, catchUpOnResume: true})
useVisiblePolling(() => updateStore.refresh(), 15_000, {enabled: () => isLocalBuild && updateStore.supported})

onMounted(() => {
	// Initial load via REST - wrapped in try/catch so the rest of setup
	// (click handler, WS subscription, polling) still runs if this fails
	void refreshNotifications(true).catch(e => console.warn('Failed to load initial notifications:', e))

	document.addEventListener('click', hidePopup)

	// Subscribe to real-time notifications
	unsubscribeWs = subscribe('notification.created', (msg) => {
		if (msg.event === 'notification.created' && msg.data) {
			const notification = new NotificationModel(msg.data as Partial<INotification>)
			// Avoid duplicates if the same notification was already loaded via REST
			const exists = allNotifications.value.some(n => n.id === notification.id)
			if (!exists && !deletedNotificationIds.has(notification.id)) {
				notificationsRevision++
				allNotifications.value = [notification, ...allNotifications.value]
			}
		}
	})

})

// Reload notifications when WebSocket disconnects to catch any events
// that may have been missed during the disconnect window
watch(wsConnected, (isConnected, wasConnected) => {
	if (wasConnected && !isConnected && !document.hidden) {
		refreshNotifications(true).catch(e => console.warn('Failed to reload notifications after WS disconnect:', e))
	}
})

onUnmounted(() => {
	document.removeEventListener('click', hidePopup)
	unsubscribeWs?.()
	disposed = true
})

function openUpdateDetails() {
	showNotifications.value = false
	showUpdateDetails.value = true
}

function openTeamConflicts() {
	showNotifications.value = false
	window.dispatchEvent(new CustomEvent('tasktrace-team-conflicts-open'))
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

function hasTeamNotificationTarget(notice: TaskTraceTeamNotification) {
	return Number.isSafeInteger(notice.task_id) && Number(notice.task_id) > 0
		&& (!notice.shared_comment_id || (Number.isSafeInteger(notice.comment_id) && Number(notice.comment_id) > 0))
}

async function openTeamNotification(notice: TaskTraceTeamNotification, archived = false) {
	if (openingTeamNotification.value) return
	openingTeamNotification.value = true
	const currentNotice = () => (!archived && notice.id && teamStore.status.notifications?.find(candidate => candidate.id === notice.id)) || notice
	async function resolvedNotice() {
		if (!archived) return currentNotice()
		const history = (await tasktraceNotificationHistoryList()).data || []
		return history.find(entry => entry.team?.id === notice.id && entry.team?.share_id === notice.share_id)?.team || notice
	}
	try {
		// The reading list is a stable snapshot. Prefer an already resolved local
		// target, then refresh status before requesting any LAN synchronization.
		let target = currentNotice()
		if (!hasTeamNotificationTarget(target) && notice.id) {
			try {
				await teamStore.refresh(true)
				target = await resolvedNotice()
				if (!hasTeamNotificationTarget(target)) {
					await teamStore.sync()
					target = await resolvedNotice()
				}
			} catch (cause) {
				showError(new Error('暂时无法读取这条成员更新，请检查团队共享目录或网络连接后重试。', {cause}))
				return
			}
		}
		if (!hasTeamNotificationTarget(target)) {
			showError({message: '暂时无法定位这条成员更新对应的事项或评论，可能尚未同步、已删除或无访问权限。请稍后重试；通知仍保留在列表中。'})
			return
		}
		const commentId = Number(target.comment_id || 0)
		const route: RouteLocationRaw = {
			name: 'task.detail',
			params: {id: target.task_id},
			...(commentId ? {hash: `#comment-${commentId}`} : {}),
		}
		const failure = await router.push(route)
		if (failure && !isNavigationFailure(failure, NavigationFailureType.duplicated)) return
		showNotifications.value = false
		if (commentId && !await waitForTeamComment(commentId)) {
			showError({message: '已打开事项，但暂时无法显示这条更新的评论。请稍后重试；通知尚未标记为已读。'})
			return
		}
		if (!archived && notice.id && !target.read) {
			await teamStore.markNotificationsRead([notice.id])
			readingTeamNotifications.value = readingTeamNotifications.value.map(item => item.id === notice.id ? {...item, read: true} : item)
		}
	} catch (cause) {
		showError(cause)
	} finally {
		openingTeamNotification.value = false
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
		success({message: '已开始更新，请在更新进度小窗中查看进度或取消。'})
	} catch (cause) {
		showError(cause)
	}
}

function displayReleaseDate(value?: string) {
	if (!value) return '发布日期未知'
	const date = new Date(value)
	return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function loadNotifications(): Promise<void> {
	if (activeRead) return activeRead
	const notificationService = new NotificationService()
	const operation = (async () => {
		while (!disposed) {
			const version = notificationsRevision
			const next: INotification[] = []
			for (let page = 1; page <= loadedPages.value; page++) {
				const batch = await notificationService.getAll(undefined, {per_page: 50}, page)
				next.push(...batch)
				totalPages.value = Math.max(1, notificationService.totalPages || 1)
				if (page >= totalPages.value || disposed) break
			}
			if (disposed) return
			// A websocket event can arrive while the initial history is being read.
			// Fetch one current snapshot instead of permanently losing the older unread items.
			if (version !== notificationsRevision) continue
			const unique = [...new Map(next.filter(item => !deletedNotificationIds.has(item.id)).map(item => [item.id, item])).values()]
			if (!equal(allNotifications.value, unique)) allNotifications.value = unique
			if (!initialReadFinished && showNotifications.value) captureNotifications()
			initialReadFinished = true
			return
		}
	})().finally(() => { if (activeRead === operation) activeRead = undefined })
	activeRead = operation
	return operation
}

function hidePopup(e: MouseEvent) {
	if (showNotifications.value && popup.value) {
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

function to(n: INotification) {
	return async () => {
		const route = getNotificationRoute(n)
		if (route === null) return
		
		const failure = await router.push(route)
		if (failure && !isNavigationFailure(failure, NavigationFailureType.duplicated)) return

		notificationsRevision++
		n.read = true
		if (allNotifications.value.some(item => item.id === n.id)) {
			const notificationService = new NotificationService()
			const saved = await notificationService.update(n)
			notificationsRevision++
			const current = allNotifications.value.find(item => item.id === n.id)
			if (current) Object.assign(current, saved)
		}

		showNotifications.value = false
	}
}

async function markAllRead() {
	notificationsRevision++
	const notificationService = new NotificationService()
	await Promise.all([
		notifications.value.some(n => n.readAt === null) ? notificationService.markAllRead() : Promise.resolve(),
		isLocalBuild && teamStore.unreadNotificationCount ? teamStore.markNotificationsRead() : Promise.resolve(),
	])
	notificationsRevision++
	success({message: t('notification.markAllReadSuccess')})

	allNotifications.value.forEach(n => n.readAt = new Date())
	readingNotifications.value.forEach(n => n.readAt = new Date())
	readingTeamNotifications.value = readingTeamNotifications.value.map(notice => ({...notice, read: true}))
}

async function clearAll() {
	if (clearingAll.value) return
	clearingAll.value = true
	notificationsRevision++
	const ids = new Set(allNotifications.value.map(notice => notice.id))
	const teamIds = (teamStore.status.notifications || []).map(notice => notice.id).filter((id): id is string => !!id)
	const notificationService = new NotificationService()
	try {
		const results = await Promise.allSettled([
			ids.size ? notificationService.delete(new NotificationModel({})).then(() => {
				ids.forEach(id => deletedNotificationIds.add(id))
				allNotifications.value = allNotifications.value.filter(notice => !ids.has(notice.id))
				readingNotifications.value = readingNotifications.value.filter(notice => !ids.has(notice.id))
			}) : Promise.resolve(),
			isLocalBuild && teamIds.length ? teamStore.dismissNotifications(teamIds).then(() => {
				readingTeamNotifications.value = readingTeamNotifications.value.filter(notice => !teamIds.includes(notice.id || ''))
			}) : Promise.resolve(),
		])
		const failures = results.filter(result => result.status === 'rejected')
		if (failures.length) failures.forEach(result => showError(result.reason))
		else success({message: isLocalBuild ? '通知已移入历史通知。' : t('notification.clearAllSuccess')})
	} finally {
		notificationsRevision++
		clearingAll.value = false
	}
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
		max-block-size: min(70dvh, 36rem);
		overscroll-behavior: contain;
		scrollbar-gutter: stable;
		overflow-y: auto;

		background: var(--white);
		color: var(--text);
		inline-size: 28rem;
		max-inline-size: calc(100vw - 2rem);
		padding: .75rem .25rem;
		border-radius: $radius;
		box-shadow: var(--shadow-sm);
		font-size: .85rem;

		@media screen and (max-width: $tablet) {
			max-block-size: calc(100vh - 1rem - #{$navbar-height});
		}

		.head {
			position: sticky;
			inset-block-start: -.75rem;
			z-index: 1;
			background: var(--white);
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
			padding: .625rem .25rem;
			border-block-end: 1px solid var(--grey-200);

			.detail {
				min-inline-size: 0;
				flex: 1;
				overflow-wrap: anywhere;
			}

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
				color: var(--grey-600);
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
		strong, .detail > span:first-child {
			overflow-wrap: anywhere;
		}
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
.reading-status {
	padding: .375rem .5rem;
	min-block-size: 2rem;
	color: var(--grey-600);
	font-size: .8rem;

	button {
		border: 0;
		padding: 0;
		background: transparent;
		color: var(--primary);
		font: inherit;
		cursor: pointer;
	}
}

.notification-delete {
	flex: 0 0 2rem;
	align-self: flex-start;
	min-block-size: 2rem;
	border: 0;
	border-radius: .375rem;
	background: transparent;
	color: var(--grey-600);
	cursor: pointer;

	&:hover {
		background: var(--grey-100);
		color: var(--danger);
	}
	&:disabled {
		cursor: wait;
		opacity: .5;
	}
}
</style>
