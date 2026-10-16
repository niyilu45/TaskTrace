import {NOTIFICATION_NAMES, type INotification} from '@/modelTypes/INotification'

export function exportNotificationPath(notice?: INotification | null): string {
	if (notice?.name !== NOTIFICATION_NAMES.EXPORT_COMPLETED) return ''
	return 'path' in notice.notification ? notice.notification.path || '' : ''
}
