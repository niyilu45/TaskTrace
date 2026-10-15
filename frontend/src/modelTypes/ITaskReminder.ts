import type { IAbstract } from './IAbstract'
import type { IReminderPeriodRelativeTo } from '@/types/IReminderPeriodRelativeTo'

export interface ITaskReminder extends IAbstract {
	repeatRule?: string
	reminder: Date | null
	relativePeriod: number
	relativeTo: IReminderPeriodRelativeTo | null
}
