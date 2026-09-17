import {test, expect, type Locator} from '@playwright/test'
import {readFileSync} from 'node:fs'
import path from 'node:path'
import {execFileSync} from 'node:child_process'

test.use({serviceWorkers: 'block'})

test('project overview columns resize, align, persist per project and reset on desktop and narrow windows', async ({page, request}) => {
	test.setTimeout(90000)
	page.setDefaultTimeout(10000)
	const root = process.env.TASKTRACE_LOCAL_TEST_DIR
	test.skip(!root, 'Requires a running isolated portable instance')
	const sessionFile = path.join(root!, 'data/column-widths-browser-session.json')
	execFileSync(path.join(root!, 'TaskTrace-server.exe'), ['--config', path.join(root!, 'data/local-config.yml'), 'tasktrace-local-session', '--output', sessionFile, '--user-id', readFileSync(path.join(root!, 'data/local-user-id.txt'), 'utf8').trim()], {windowsHide: true, stdio: 'pipe'})
	const session = JSON.parse(readFileSync(sessionFile, 'utf8').replace(/^\uFEFF/, ''))
	const base = readFileSync(path.join(root!, 'data/local-config.yml'), 'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
	const headers = {Authorization: 'Bearer ' + session.token}
	const projectIds: number[] = []
	async function create(url: string, data: object) {
		const response = await request.post(base + '/api/v2' + url, {headers, data})
		if (!response.ok()) throw new Error(`${url}: HTTP ${response.status()} ${await response.text()}`)
		return response.json()
	}
	async function project(title: string) {
		const result = await create('/projects', {title})
		projectIds.push(result.id)
		return result
	}
	async function addTask(projectId: number, title: string, description: string, parentId?: number) {
		const task = await create(`/projects/${projectId}/tasks`, {title, description: `<p>${description}</p>`})
		if (parentId) await create(`/tasks/${parentId}/relations`, {other_task_id: task.id, relation_kind: 'subtask'})
		await create(`/tasks/${task.id}/comments`, {comment: `<h3>每日进展 · 2026-09-17</h3><p>已完成${title}初步整理，下一步确认细节。</p>`})
		await create(`/tasks/${task.id}/comments`, {comment: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="check-${task.id}">等待确认验收标准</li></ul>`})
		return task
	}
	const overview = page.getByRole('region', {name: '项目展示模式', exact: true})
	const visibleTables = overview.locator('.subtask-table:visible')
	const childTables = overview.locator('.progress-group > .subtask-scroll .subtask-table')
	const reset = overview.getByRole('button', {name: /恢复默认列宽/})
	async function expandOwnProgress() {
		for (const details of await overview.locator('.task-own-progress').all()) {
			if (!(await details.evaluate(el => (el as HTMLDetailsElement).open))) await details.locator('summary').click()
		}
	}
	async function widths(table: Locator) {
		return table.getByRole('separator').evaluateAll(handles => handles.map(handle => Number(handle.getAttribute('aria-valuenow'))))
	}
	async function assertAligned(expected: number[]) {
		const measurements = await visibleTables.evaluateAll(tables => tables.map(table => ({
			header: [...table.querySelectorAll('thead th')].map(cell => ({x: cell.getBoundingClientRect().x, width: cell.getBoundingClientRect().width})),
			row: [...(table.querySelector('tbody tr')?.children || [])].map(cell => ({x: cell.getBoundingClientRect().x, width: cell.getBoundingClientRect().width})),
		})))
		expect(measurements.length).toBeGreaterThanOrEqual(4)
		for (const table of measurements) {
			expect(table.header).toHaveLength(4)
			expect(table.row).toHaveLength(4)
			for (let index = 0; index < 4; index++) {
				expect(Math.abs(table.header[index].width - expected[index])).toBeLessThanOrEqual(2)
				expect(Math.abs(table.row[index].width - table.header[index].width)).toBeLessThanOrEqual(1)
				expect(Math.abs(table.header[index].x - measurements[0].header[index].x)).toBeLessThanOrEqual(2)
			}
		}
	}
	async function dragColumn(handle: Locator, distance: number) {
		await handle.scrollIntoViewIfNeeded()
		const box = await handle.boundingBox()
		expect(box).not.toBeNull()
		const x = box!.x + box!.width / 2
		const y = box!.y + box!.height / 2
		await page.mouse.move(x, y)
		await page.mouse.down()
		await page.mouse.move(x + distance, y, {steps: 8})
		await page.mouse.up()
	}
	async function resetScrollPositions() {
		await overview.locator('.subtask-scroll').evaluateAll(elements => elements.forEach(element => { element.scrollLeft = 0 }))
	}
	try {
		let first: {id: number}, second: {id: number}, deepest: {id: number}
		// Reuse isolated fixtures when diagnosing browser behavior independently of API writes.
		const reuse = process.env.TASKTRACE_COLUMN_WIDTH_FIXTURES?.split(',').map(Number)
		if (reuse?.length === 3 && reuse.every(id => Number.isSafeInteger(id) && id > 0)) {
			[first, second, deepest] = reuse.map(id => ({id}))
		} else {
			first = await project('列宽调整验收')
			second = await project('独立列宽验收')
			const design = await addTask(first.id, '产品设计', '整理界面布局、操作步骤与验收标准，较长描述用于验证可调列宽。')
			const requirements = await addTask(first.id, '需求梳理', '确认事项、子任务和每日进展的展示规则。', design.id)
			const boundaries = await addTask(first.id, '边界核对', '检查多级子任务的表格列保持对齐。', requirements.id)
			const review = await addTask(first.id, '评审确认', '确认第四级任务的缩进。', boundaries.id)
			deepest = await addTask(first.id, '五级子任务名称仍需完整阅读', '名称缩窄后不能遮挡本列描述。', review.id)
			const delivery = await addTask(first.id, '开发交付', '完成实现后进行演示和验收。')
			await addTask(first.id, '功能验收', '检查拖动调整、键盘操作和刷新后的保存结果。', delivery.id)
			const separate = await addTask(second.id, '独立项目任务', '此项目应保留自己的列宽设置。')
			await addTask(second.id, '独立子任务', '不能继承其他项目调整后的列宽。', separate.id)
		}
		console.info('Column widths: fixtures ready, checking desktop interactions')
		await page.setViewportSize({width: 1440, height: 1000})
		await page.goto(`${base}/projects/${first.id}#tasktrace-local=` + encodeURIComponent(JSON.stringify(session)))
		await expect(overview).toBeVisible()
		await expect(page.locator('.vikunja-loading')).toBeHidden()
		await expect(childTables).toHaveCount(2)
		await expandOwnProgress()
		await expect(visibleTables).toHaveCount(4)
		const table = childTables.first()
		const handles = table.getByRole('separator')
		await expect(handles).toHaveCount(4)
		for (const [index, handle] of (await handles.all()).entries()) {
			await expect(handle).toHaveAttribute('aria-orientation', 'vertical')
			await expect(handle).toHaveAttribute('aria-valuemin', index === 0 ? '144' : '96')
			await expect(handle).toHaveAttribute('aria-valuemax', '1600')
			await expect(handle).toHaveAccessibleName(/列宽/)
		}
		const defaults = await widths(table)
		expect(defaults.every((width, index) => width >= (index === 0 ? 144 : 96) && width <= 1600)).toBeTruthy()
		await assertAligned(defaults)
		await dragColumn(handles.nth(1), 120)
		await expect.poll(async () => (await widths(table))[1]).toBe(defaults[1] + 120)
		const dragged = await widths(table)
		for (const index of [0, 2, 3]) expect(dragged[index]).toBe(defaults[index])
		await resetScrollPositions()
		await assertAligned(dragged)
		// Each column, including the final one, supports keyboard resizing.
		for (let index = 0; index < 4; index++) {
			const before = (await widths(table))[index]
			await handles.nth(index).press('ArrowRight')
			await expect.poll(async () => (await widths(table))[index]).toBeGreaterThan(before)
			await handles.nth(index).press('ArrowLeft')
			await expect.poll(async () => (await widths(table))[index]).toBe(before)
		}
		await dragColumn(handles.nth(3), 100)
		await expect.poll(async () => (await widths(table))[3]).toBe(defaults[3] + 100)
		const saved = await widths(table)
		await page.reload()
		await expect(childTables).toHaveCount(2)
		await expandOwnProgress()
		await expect.poll(() => widths(table)).toEqual(saved)
		await assertAligned(saved)
		await page.goto(`${base}/projects/${second.id}`)
		await expect(childTables).toHaveCount(1)
		await expect.poll(() => widths(childTables.first())).toEqual(defaults)
		await page.goto(`${base}/projects/${first.id}`)
		await expect(childTables).toHaveCount(2)
		await expandOwnProgress()
		await expect.poll(() => widths(table)).toEqual(saved)
		console.info('Column widths: persistence and project isolation passed, checking limits')
		// Pointer movement beyond the range clamps at the documented limits.
		await dragColumn(handles.nth(0), -3000)
		await expect.poll(async () => (await widths(table))[0]).toBe(144)
		const deepRow = table.locator(`[data-task-id="${deepest.id}"]`)
		const deepBounds = await deepRow.evaluate(row => {
			const name = row.querySelector('.task-name > span:last-child')!.getBoundingClientRect()
			const description = row.children[1].getBoundingClientRect()
			return {nameRight: name.right, descriptionLeft: description.left}
		})
		expect(deepBounds.nameRight).toBeLessThanOrEqual(deepBounds.descriptionLeft)
		await dragColumn(handles.nth(0), 5000)
		await expect.poll(async () => (await widths(table))[0]).toBe(1600)
		await handles.nth(0).press('ArrowRight')
		await expect(handles.nth(0)).toHaveAttribute('aria-valuenow', '1600')
		await expect.poll(() => table.locator('..').evaluate(element => element.scrollWidth > element.clientWidth)).toBeTruthy()
		await reset.click()
		await expect.poll(() => widths(table)).toEqual(defaults)
		await page.reload()
		await expect(childTables).toHaveCount(2)
		await expandOwnProgress()
		await expect.poll(() => widths(table)).toEqual(defaults)
		await assertAligned(defaults)
		await dragColumn(handles.nth(1), 100)
		await resetScrollPositions()
		await assertAligned(await widths(table))
		await page.evaluate(() => window.scrollTo(0, 0))
		await page.screenshot({path: path.join(root!, 'column-widths-desktop.png'), fullPage: true, animations: 'disabled'})
		console.info('Column widths: desktop complete, checking narrow-screen scrolling')
		await page.setViewportSize({width: 390, height: 844})
		const menuButton = page.getByRole('button', {name: '显示菜单', exact: true})
		await expect(menuButton).toHaveAttribute('aria-expanded', 'false')
		await expect(page.locator('.vikunja-loading')).toBeHidden()
		await resetScrollPositions()
		await assertAligned(await widths(table))
		await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy()
		const scroll = table.locator('..')
		await expect.poll(() => scroll.evaluate(element => element.scrollWidth > element.clientWidth)).toBeTruthy()
		const installBannerClose = page.getByRole('button', {name: 'Close banner', exact: true})
		if (await installBannerClose.isVisible()) await installBannerClose.click()
		await table.locator('thead th').first().hover()
		await page.mouse.wheel(280, 0)
		await expect.poll(() => scroll.evaluate(element => element.scrollLeft)).toBeGreaterThan(0)
		await resetScrollPositions()
		await page.evaluate(() => window.scrollTo(0, 0))
		await page.screenshot({path: path.join(root!, 'column-widths-mobile.png'), fullPage: true, animations: 'disabled'})
	} finally {
		for (const id of projectIds.reverse()) await request.delete(`${base}/api/v2/projects/${id}`, {headers}).catch(() => undefined)
	}
})
