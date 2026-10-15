import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {createDefaultViews} from '../project/prepareProjects'

test.skip(process.env.VITE_TASKTRACE_LOCAL !== 'true', 'Requires the local frontend build')

const status = {enabled: true, bindings: [], conflicts: [], notifications: []}
const sharedLink = 'tasktrace-team://test-shared-link'

test('overview imports into an existing project, refreshes tasks, and removes obsolete entry points', async ({authenticatedPage: page, apiContext, userToken}, testInfo) => {
	await ProjectFactory.create(1, {id: 1, title: '原有项目'})
	await ProjectFactory.create(1, {id: 2, title: '接收共享任务的项目'}, false)
	await createDefaultViews(1)
	await createDefaultViews(2, 5)
	await page.route('**/api/v2/tasktrace/team/status', route => route.fulfill({json: status}))
	const api = (process.env.API_URL || '').replace(/\/api\/v1\/?$/, '/api/v2')
	const requests: {link: string, project_id: number}[] = []
	let finishImport = () => {}
	const pending = new Promise<void>(resolve => { finishImport = resolve })
	await page.route('**/api/v2/tasktrace/team/import', async route => {
		const payload = route.request().postDataJSON()
		requests.push(payload)
		await pending
		const created = await apiContext.post(api + '/projects/' + payload.project_id + '/tasks', {
			headers: {Authorization: 'Bearer ' + userToken}, data: {title: '刚导入的共享任务'},
		})
		expect(created.ok()).toBe(true)
		await route.fulfill({json: status})
	})
	await page.goto('/')
	await expect(page.locator('.task-add')).toHaveCount(0)
	await page.getByRole('button', {name: '导入共享任务', exact: true}).click()
	const dialog = page.getByRole('dialog', {name: '导入团队任务链接'})
	const target = dialog.getByLabel('保存到项目')
	await expect(target.locator('option')).toContainText(['请选择项目', '原有项目', '接收共享任务的项目', '新建项目'])
	await dialog.getByLabel('任务链接', {exact: true}).fill(sharedLink)
	await target.selectOption('2')
	await expect(dialog).toHaveCSS('opacity', '1')
	await dialog.screenshot({path: testInfo.outputPath('existing-project-import.png')})
	await dialog.getByRole('button', {name: '导入任务', exact: true}).click()
	await expect(target).toBeDisabled()
	await dialog.getByRole('button', {name: '正在导入…'}).click()
	await expect.poll(() => requests.length).toBe(1)
	finishImport()
	await expect(dialog).toBeHidden()
	expect(requests[0]).toEqual({link: sharedLink, project_id: 2})
	await expect(page.locator('[data-overview-project="2"]')).toContainText('刚导入的共享任务')
	await expect(page).toHaveURL(/\/$/)
	await page.goto('/projects/1/1?mode=edit')
	await page.getByRole('button', {name: '导入共享任务链接', exact: true}).click()
	await expect(dialog.getByLabel('保存到项目')).toHaveValue('1')
	await dialog.getByRole('button', {name: /^(Cancel|取消)$/}).click()
	await expect(dialog).toBeHidden()
	await page.goto('/teams')
	await expect(page.getByRole('heading', {name: '所有协作人员'})).toBeVisible()
	await expect(page.getByRole('button', {name: /导入任务链接|导入共享任务/})).toHaveCount(0)
})

test('overview without projects can create a destination, and retries reuse it after a failed import', async ({authenticatedPage: page, apiContext, userToken}, testInfo) => {
	const api = (process.env.API_URL || '').replace(/\/api\/v1\/?$/, '/api/v2')
	const headers = {Authorization: 'Bearer ' + userToken}
	let creates = 0
	page.on('request', request => {
		if (request.method() === 'POST' && request.url().endsWith('/api/v2/projects')) creates++
	})
	await page.route('**/api/v2/tasktrace/team/status', route => route.fulfill({json: status}))
	const imports: {project_id: number}[] = []
	await page.route('**/api/v2/tasktrace/team/import', async route => {
		const payload = route.request().postDataJSON()
		imports.push(payload)
		if (imports.length === 1) {
			await route.fulfill({status: 503, json: {detail: '共享目录暂时不可用，请重试。'}})
			return
		}
		const task = await apiContext.post(api + '/projects/' + payload.project_id + '/tasks', {
			headers, data: {title: '新项目中的共享任务'},
		})
		expect(task.ok()).toBe(true)
		await route.fulfill({json: status})
	})
	await page.goto('/')
	await page.getByRole('button', {name: '导入共享任务', exact: true}).click()
	const dialog = page.getByRole('dialog', {name: '导入团队任务链接'})
	await expect(dialog.getByLabel('保存到项目')).toHaveValue('new')
	await dialog.getByRole('button', {name: '导入任务', exact: true}).click()
	await expect(dialog.getByRole('alert')).toContainText('请先粘贴共享任务链接')
	expect(creates).toBe(0)
	await dialog.getByLabel('任务链接', {exact: true}).fill(sharedLink)
	await dialog.getByRole('button', {name: '导入任务', exact: true}).click()
	await expect(dialog.getByRole('alert')).toContainText('请输入新项目名称')
	expect(creates).toBe(0)
	await dialog.getByLabel('新项目名称').fill('新建的协作项目')
	await page.setViewportSize({width: 390, height: 844})
	await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
	await page.screenshot({path: testInfo.outputPath('new-project-mobile.png'), fullPage: true})
	await dialog.getByRole('button', {name: /^(Cancel|取消)$/}).click()
	await expect(dialog).toBeHidden()
	expect(creates).toBe(0)
	await page.setViewportSize({width: 1280, height: 900})
	await page.getByRole('button', {name: '导入共享任务', exact: true}).click()
	await expect(dialog.getByLabel('新项目名称')).toHaveValue('')
	await dialog.getByLabel('任务链接', {exact: true}).fill(sharedLink)
	await dialog.getByLabel('新项目名称').fill('新建的协作项目')
	await expect(dialog).toHaveCSS('opacity', '1')
	await dialog.screenshot({path: testInfo.outputPath('new-project-import.png')})
	await dialog.getByRole('button', {name: '导入任务', exact: true}).click()
	await expect(dialog.getByRole('alert')).toContainText('共享目录暂时不可用')
	expect(creates).toBe(1)
	await expect(dialog.getByLabel('任务链接', {exact: true})).toHaveValue(sharedLink)
	await expect(dialog.getByLabel('保存到项目')).toHaveValue(String(imports[0].project_id))
	await expect(dialog.getByLabel('新项目名称')).toHaveCount(0)
	await dialog.getByRole('button', {name: '导入任务', exact: true}).click()
	await expect(dialog).toBeHidden()
	expect(creates).toBe(1)
	expect(imports).toHaveLength(2)
	expect(imports[1].project_id).toBe(imports[0].project_id)
	await expect(page.locator('[data-overview-project]')).toContainText('新项目中的共享任务')
	await page.reload()
	await expect(page.locator('[data-overview-project]')).toContainText('新建的协作项目')
	await expect(page.locator('[data-overview-project]')).toContainText('新项目中的共享任务')
})
