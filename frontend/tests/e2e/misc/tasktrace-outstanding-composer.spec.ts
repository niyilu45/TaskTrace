import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'

test('outstanding composer keeps actions inline and preserves collapsed notes', async ({authenticatedPage: page, currentUser}, testInfo) => {
	const [project] = await ProjectFactory.create(1, {owner_id: currentUser.id})
	const [task] = await TaskFactory.create(1, {project_id: project.id, created_by_id: currentUser.id})
	await page.goto(`/tasks/${task.id}`)
	const shared = page.getByRole('region', {name: '共享遗留事项'})
	await shared.getByRole('button', {name: '新增遗留事项', exact: true}).click()
	const header = shared.locator('.outstanding-composer-header')
	const note = shared.locator('.outstanding-note-editor [contenteditable="true"]')
	await expect(note).toHaveCount(0)
	await expect(shared.getByRole('combobox')).toHaveValue('7')
	await expect(shared.getByRole('button', {name: '选择图片', exact: true})).toHaveCount(0)
	await shared.getByRole('textbox', {name: '新增遗留事项', exact: true}).fill('等待确认设备复测结果')
	await shared.getByRole('combobox').selectOption('2')
	for (const width of [1280, 390]) {
		await page.setViewportSize({width, height: 900})
		await header.scrollIntoViewIfNeeded()
		const bounds = await header.locator(':scope > *').evaluateAll(elements => elements.map(element => {
			const rect = element.getBoundingClientRect()
			return {left: rect.left, right: rect.right, center: rect.top + rect.height / 2}
		}))
		expect(Math.max(...bounds.map(rect => rect.center)) - Math.min(...bounds.map(rect => rect.center))).toBeLessThan(2)
		expect(bounds[0].right).toBeLessThanOrEqual(bounds[1].left)
		expect(bounds[1].right).toBeLessThanOrEqual(bounds[2].left)
		expect(bounds[2].right).toBeLessThanOrEqual(width)
		await shared.locator('.outstanding-composer').screenshot({path: testInfo.outputPath(`composer-${width}.png`)})
	}
	await shared.getByRole('button', {name: '添加遗留事项备注', exact: true}).click()
	await note.fill('备注中的处理说明需要保留')
	await shared.getByRole('button', {name: '收起遗留事项备注', exact: true}).click()
	await header.getByRole('button', {name: '添加遗留事项', exact: true}).click()
	await expect(shared.locator('.outstanding-list')).toContainText('等待确认设备复测结果')
	await page.reload()
	await shared.getByRole('button', {name: '编辑', exact: true}).click()
	await expect(note).toHaveCount(0)
	await expect(shared.getByRole('combobox')).toHaveValue('2')
	await shared.getByRole('button', {name: '查看/编辑遗留事项备注', exact: true}).click()
	await expect(note).toContainText('备注中的处理说明需要保留')
	await shared.locator('.outstanding-composer').screenshot({path: testInfo.outputPath('composer-note-expanded.png')})
})
