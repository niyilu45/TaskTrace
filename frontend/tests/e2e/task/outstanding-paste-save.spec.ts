import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'
import {TaskCommentFactory} from '../../factories/task_comment'

test('preserves pasted line breaks and saves a new outstanding note without a second priority write', async ({authenticatedPage: page}) => {
	await ProjectFactory.create(1)
	await TaskFactory.create(1, {id: 1, title: '遗留事项保存验证'})
	await TaskCommentFactory.create(1, {id: 1, task_id: 1, comment: '<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul><li data-id="original" data-priority="7"><p>已有遗留事项</p></li></ul>'})
	let priorityRequests = 0
	await page.route('**/api/v2/tasktrace/tasks/1/priority', async route => {
		priorityRequests++
		await route.fulfill({status: 409, contentType: 'application/json', body: JSON.stringify({detail: 'Concurrent personal-priority update'})})
	})
	await page.goto('/tasks/1')
	const outstanding = page.locator('.shared-outstanding')
	await outstanding.getByRole('button', {name: '新增遗留事项', exact: true}).click()
	await outstanding.locator('textarea').fill('成员新增验证')
	await outstanding.locator('.outstanding-priority-select').selectOption('2')
	await outstanding.getByRole('button', {name: '添加遗留事项备注', exact: true}).click()
	const editor = outstanding.locator('.outstanding-note-editor .tiptap.ProseMirror')
	await expect(editor).toBeVisible()
	await editor.click()
	await editor.evaluate(element => {
		const data = new DataTransfer()
		data.setData('text/plain', '甲\r\n乙\r\n\r\n丙')
		element.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true}))
	})
	await expect(editor).toHaveJSProperty('innerHTML', '<p>甲<br>乙<br><br>丙</p>')
	const image = await page.evaluate(() => {
		const canvas = document.createElement('canvas')
		canvas.width = 16; canvas.height = 16
		canvas.getContext('2d')!.fillRect(0, 0, 16, 16)
		return canvas.toDataURL('image/png')
	})
	await editor.evaluate((element, source) => {
		const data = new DataTransfer()
		data.setData('text/html', '<p class="MsoNormal" style="color:red;font-size:40pt">图文前\r\n<br>\r\n<img src="' + source + '">图文后</p>')
		data.setData('text/plain', '图文前\n图文后')
		element.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true}))
	}, image)
	await expect(editor).toContainText('图文后')
	await expect(editor.locator('img')).toHaveCount(1)
	await expect(editor).not.toHaveAttribute('style', /color|font-size/)
	await outstanding.getByRole('button', {name: '添加遗留事项', exact: true}).click()
	await expect(outstanding).toContainText('遗留事项已保存。')
	await expect(outstanding.locator('[role="alert"]')).toHaveCount(0)
	expect(priorityRequests).toBe(0)
	await page.reload()
	const row = outstanding.locator('.outstanding-list > li').filter({hasText: '成员新增验证'})
	await expect(row).toContainText('P2')
	await expect(outstanding).toContainText('已有遗留事项')
	await row.getByRole('button', {name: '编辑', exact: true}).click()
	await outstanding.getByRole('button', {name: '查看/编辑遗留事项备注', exact: true}).click()
	await expect(editor).toContainText('甲')
	await expect(editor).toContainText('图文后')
	await expect(editor.locator('img')).toHaveCount(1)
	await expect(editor.locator('p:empty')).toHaveCount(0)
	await page.screenshot({path: test.info().outputPath('outstanding-paste-saved.png'), fullPage: true})
})
