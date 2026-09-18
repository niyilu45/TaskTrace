import {beforeEach, describe, expect, it} from 'vitest'
import {mount} from '@vue/test-utils'
import {createPinia, setActivePinia} from 'pinia'
import {createI18n} from 'vue-i18n'
import CalendarMonth from './CalendarMonth.vue'
import ProgressDatePicker from '@/components/tasks/partials/ProgressDatePicker.vue'
import en from '@/i18n/lang/en.json'

const i18n = createI18n({legacy: false, locale: 'en', messages: {en}})

describe('CalendarMonth progress markers', () => {
	beforeEach(() => setActivePinia(createPinia()))

	it('marks only dates with saved progress and describes the marker', () => {
		const wrapper = mount(CalendarMonth, {
			props: {
				selected: new Date(2026, 8, 18),
				markedDates: ['2026-09-17', '2026-09-18'],
			},
			global: {plugins: [i18n], stubs: {BaseButton: true, Icon: true}},
		})
		const marked = wrapper.findAll('.calendar-month__day.has-marker')
		expect(marked).toHaveLength(2)
		expect(wrapper.get('[data-date="2026-09-17"]').attributes('aria-label')).toContain('有进展')
		expect(wrapper.get('[data-date="2026-09-16"]').classes()).not.toContain('has-marker')
	})

	it('shows the selected month beside its previous and next months', async () => {
		const wrapper = mount(ProgressDatePicker, {
			props: {
				id: 'progress-date',
				modelValue: '2026-09-18',
				markedDates: ['2026-08-31', '2026-09-18', '2026-10-02'],
			},
			global: {
				plugins: [i18n],
				stubs: {Icon: true},
				directives: {tooltip: () => undefined},
			},
		})
		await wrapper.get('.progress-date-picker__trigger').trigger('click')
		expect(wrapper.findAll('[data-month]').map(month => month.attributes('data-month')))
			.toEqual(['2026-08', '2026-09', '2026-10'])
		expect(wrapper.findAll('.calendar-month__day.has-marker')).toHaveLength(3)

		await wrapper.findAll('.calendar-month__nav-button')[0].trigger('click')
		expect(wrapper.findAll('[data-month]').map(month => month.attributes('data-month')))
			.toEqual(['2026-07', '2026-08', '2026-09'])
	})

	it('closes without changing the date when the user clicks outside', async () => {
		const wrapper = mount(ProgressDatePicker, {
			attachTo: document.body,
			props: {id: 'progress-date-outside', modelValue: '2026-09-18', markedDates: ['2026-09-18']},
			global: {plugins: [i18n], stubs: {Icon: true}, directives: {tooltip: () => undefined}},
		})
		await wrapper.get('.progress-date-picker__trigger').trigger('click')
		expect(wrapper.find('.progress-date-picker__popup').exists()).toBe(true)
		document.body.dispatchEvent(new MouseEvent('pointerdown', {bubbles: true}))
		await wrapper.vm.$nextTick()
		expect(wrapper.find('.progress-date-picker__popup').exists()).toBe(false)
		expect(wrapper.emitted('update:modelValue')).toBeUndefined()
		wrapper.unmount()
	})
})
