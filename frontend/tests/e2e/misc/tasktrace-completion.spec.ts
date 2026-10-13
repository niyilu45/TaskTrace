import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'

test('manual completion survives done overlay, external updates and reload', async ({authenticatedPage: page, apiContext, userToken, currentUser}, testInfo) => {
	const [project] = await ProjectFactory.create(1, {owner_id: currentUser.id})
	const [task] = await TaskFactory.create(1, {project_id: project.id, created_by_id: currentUser.id, title: '完成度验证', percent_done: 0})
	const headers = {Authorization: `Bearer ${userToken}`}
	const body = '<h3 data-tasktrace-comment-type="outstanding">Outstanding items</h3><ul><li data-id="one" data-priority="7" data-done="false">等待设备复测</li></ul>'
	const created = await apiContext.post(`../v2/tasks/${task.id}/comments`, {headers, data: {comment: body}})
	expect(created.ok()).toBeTruthy()
	const comment = await created.json()
	await page.goto(`/tasks/${task.id}`)
	const percent = page.getByRole('combobox', {name: '填写完成度，勾选完成不会清除填写值'})
	await expect(percent).toHaveValue('0')
	await percent.selectOption('0.4')
	await expect(percent).toBeEnabled()
	const read = async () => (await apiContext.get(`../v2/tasks/${task.id}`, {headers})).json()
	await expect.poll(async () => (await read()).percent_done).toBe(0.4)
	const shared = page.getByRole('region', {name: '共享遗留事项'})
	const itemPercent = shared.getByRole('combobox', {name: '第 1 条遗留事项的填写完成度'})
	await expect(itemPercent).toHaveValue('0')
	await itemPercent.selectOption('60')
	await expect(shared).toContainText('60%')
	await page.reload()
	await expect(percent).toHaveValue('0.4')
	await expect(itemPercent).toHaveValue('60')
	const change = async (done: boolean) => {
		const response = await apiContext.patch(`../v2/tasks/${task.id}`, {headers: {...headers, 'Content-Type': 'application/merge-patch+json'}, data: {done}})
		expect(response.ok()).toBeTruthy()
	}
	await change(true)
	await expect(page.getByText('当前完成度：100%（已勾选完成）')).toBeVisible()
	await expect(percent).toHaveValue('0.4')
	await shared.getByRole('checkbox', {name: '完成第 1 条遗留事项'}).check()
	await expect(shared).toContainText('100%')
	await expect(itemPercent).toHaveValue('60')
	for (const width of [1280, 390]) {
		await page.setViewportSize({width, height: 1000})
		await percent.locator('..').locator('..').locator('..').screenshot({path: testInfo.outputPath(`task-completion-${width}.png`)})
		await shared.screenshot({path: testInfo.outputPath(`outstanding-completion-${width}.png`)})
	}
	await change(false)
	await expect(page.getByText('当前完成度：100%（已勾选完成）')).toHaveCount(0)
	await expect(percent).toHaveValue('0.4')
	const external = body.replace('data-done="false"', 'data-done="false" data-progress="80"')
	expect((await apiContext.put(`../v2/tasks/${task.id}/comments/${comment.id}`, {headers, data: {comment: external}})).ok()).toBeTruthy()
	await expect(itemPercent).toHaveValue('80')
	await expect(shared).toContainText('80%')
	await page.reload()
	await expect(itemPercent).toHaveValue('80')
	await expect(percent).toHaveValue('0.4')
})
