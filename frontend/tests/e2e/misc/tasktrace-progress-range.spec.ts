import {test, expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import path from 'node:path'
import {execFileSync} from 'node:child_process'
import {setTimeout as delay} from 'node:timers/promises'

test.use({serviceWorkers: 'block'})

type Note = {date: string, text: string}
type TrackedTask = {id: number, title: string, description: string, notes: Note[]}

test('progress range uses each task latest date, keeps calendar boundaries and persists per project', async ({page, request}) => {
	test.setTimeout(90000)
	page.setDefaultTimeout(10000)
	const root = process.env.TASKTRACE_LOCAL_TEST_DIR
	test.skip(!root, 'Requires a running isolated portable test instance')
	const sessionPath = path.join(root!, 'data/progress-range-browser-session.json')
	execFileSync(path.join(root!, 'TaskTrace-server.exe'), ['--config', path.join(root!, 'data/local-config.yml'), 'tasktrace-local-session', '--output', sessionPath, '--user-id', readFileSync(path.join(root!, 'data/local-user-id.txt'), 'utf8').trim()], {windowsHide: true, stdio: 'pipe'})
	const session = JSON.parse(readFileSync(sessionPath, 'utf8').replace(/^\uFEFF/, ''))
	const base = readFileSync(path.join(root!, 'data/local-config.yml'), 'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
	const headers = {Authorization: 'Bearer ' + session.token, 'X-TaskTrace-Undo': '1'}
	const projectIds: number[] = []
	async function create(url: string, data: object) {
		const response = await request.post(base + '/api/v2' + url, {headers, data})
		if (!response.ok()) throw new Error(`${url}: HTTP ${response.status()} ${await response.text()}`)
		const result = await response.json()
		// Match local UI journal writes and allow asynchronous fixture events to finish.
		await delay(100)
		return result
	}
	async function addProject(title: string) {
		const project = await create('/projects', {title})
		projectIds.push(project.id)
		return project
	}
	async function addTask(projectId: number, title: string, notes: Note[], parentId?: number): Promise<TrackedTask> {
		const description = `${title}的说明始终显示，不随进展范围变化。`
		const task = await create(`/projects/${projectId}/tasks`, {title, description: `<p>${description}</p>`})
		if (parentId) await create(`/tasks/${parentId}/relations`, {other_task_id: task.id, relation_kind: 'subtask'})
		for (const note of notes) await create(`/tasks/${task.id}/comments`, {comment: `<h3>每日进展 · ${note.date}</h3><p>${note.text}</p>`})
		if (notes.length) await create(`/tasks/${task.id}/comments`, {comment: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="range-${task.id}">${title}待确认事项始终保留</li></ul>`})
		return {id: task.id, title, description, notes}
	}
	const overview = page.getByRole('region', {name: '项目展示模式', exact: true})
	const range = overview.getByRole('combobox', {name: '进展显示范围', exact: true})
	const days = overview.getByRole('spinbutton', {name: '进展显示天数', exact: true})
	function row(task: TrackedTask) { return overview.locator(`[data-task-id="${task.id}"]`) }
	async function expandOwnProgress() {
		for (const details of await overview.locator('.task-own-progress').all()) {
			if (!(await details.evaluate(element => (element as HTMLDetailsElement).open))) await details.locator('summary').click()
		}
	}
	async function assertNotes(task: TrackedTask, dates: string[]) {
		const target = row(task)
		await target.scrollIntoViewIfNeeded()
		const included = task.notes.filter(note => dates.includes(note.date))
		const entries = target.locator('.progress-cell .history-entry')
		await expect(entries).toHaveCount(included.length)
		for (const note of included) await expect(target.locator('.progress-cell')).toContainText(note.text)
		for (const note of task.notes.filter(note => !dates.includes(note.date))) await expect(target.locator('.progress-cell')).not.toContainText(note.text)
		await expect(target.getByRole('rowheader')).toContainText(task.title)
		await expect(target.locator('td').nth(0)).toContainText(task.description)
		await expect(target.locator('.outstanding-cell')).toContainText(`${task.title}待确认事项始终保留`)
	}
	async function assertHidden(task: TrackedTask, count: number) {
		await expect(row(task).locator('.progress-cell')).toContainText(`已隐藏 ${count} 条较早进展`)
	}
	try {
		const project = await addProject('进展范围验收')
		const otherProject = await addProject('独立范围设置验收')
		const parent = await addTask(project.id, '产品规划', [
			{date: '2022-05-20', text: '最新当天：完成需求梳理'},
			{date: '2022-05-20', text: '最新当天：补充评审结论'},
			{date: '2022-05-18', text: '近三天边界：确认交付范围'},
			{date: '2022-05-14', text: '近七天边界：形成初版方案'},
			{date: '2022-05-13', text: '超出七天：记录早期讨论'},
			{date: '2022-04-01', text: '超出三十天：项目准备'},
		])
		const child = await addTask(project.id, '交互设计', [
			{date: '2021-11-09', text: '设计最近一天：确认原型'},
			{date: '2021-11-07', text: '设计近三天：调整按钮'},
			{date: '2021-11-03', text: '设计近七天：整理流程'},
			{date: '2021-11-02', text: '设计更早进展：初步调研'},
		], parent.id)
		const third = await addTask(project.id, '三级检查', [], child.id)
		const fourth = await addTask(project.id, '四级复核', [], third.id)
		const deepest = await addTask(project.id, '五级验收', [
			{date: '2019-02-28', text: '五级最近一天：验收确认'},
			{date: '2019-02-26', text: '五级近三天：修正细节'},
			{date: '2019-02-22', text: '五级近七天：完成核对'},
			{date: '2019-02-21', text: '五级更早进展：列出检查点'},
		], fourth.id)
		const tracked = [parent, child, deepest]
		const assertAll = async () => { for (const task of tracked) await assertNotes(task, task.notes.map(note => note.date)) }
		const assertThreeDays = async () => {
			await assertNotes(parent, ['2022-05-20', '2022-05-18'])
			await assertNotes(child, ['2021-11-09', '2021-11-07'])
			await assertNotes(deepest, ['2019-02-28', '2019-02-26'])
		}
		console.info('Progress range: fixtures ready, checking default and date boundaries')
		await page.setViewportSize({width: 1440, height: 1000})
		await page.goto(`${base}/projects/${project.id}#tasktrace-local=` + encodeURIComponent(JSON.stringify(session)))
		await expect(overview).toBeVisible()
		await expect(page.locator('.vikunja-loading')).toBeHidden()
		await expect(range).toHaveValue('all')
		await expect(range.locator('option')).toHaveText(['全部', '最近 1 天', '最近 7 天', '最近 30 天', '自定义'])
		await expandOwnProgress()
		await assertAll()
		await range.selectOption('7')
		await assertNotes(parent, ['2022-05-20', '2022-05-18', '2022-05-14'])
		await assertNotes(child, ['2021-11-09', '2021-11-07', '2021-11-03'])
		await assertNotes(deepest, ['2019-02-28', '2019-02-26', '2019-02-22'])
		await assertHidden(parent, 2)
		await assertHidden(child, 1)
		await assertHidden(deepest, 1)
		await range.selectOption('1')
		await assertNotes(parent, ['2022-05-20'])
		await assertNotes(child, ['2021-11-09'])
		await assertNotes(deepest, ['2019-02-28'])
		await range.selectOption('30')
		await assertNotes(parent, ['2022-05-20', '2022-05-18', '2022-05-14', '2022-05-13'])
		await range.selectOption('custom')
		await expect(days).toHaveValue('7')
		await expect(days).toHaveAttribute('min', '1')
		await expect(days).toHaveAttribute('max', '36500')
		await days.fill('3')
		// A draft value does not alter the applied range until change/blur.
		await expect(row(parent).locator('.progress-cell .history-entry')).toHaveCount(4)
		await days.press('Tab')
		await assertThreeDays()
		await days.fill('0')
		await days.press('Tab')
		await assertThreeDays()
		await days.fill('3')
		await days.press('Enter')
		console.info('Progress range: boundary checks passed, checking persistence and project isolation')
		await page.reload()
		await expect(range).toHaveValue('custom')
		await expect(days).toHaveValue('3')
		await expandOwnProgress()
		await assertThreeDays()
		await page.goto(`${base}/projects/${otherProject.id}`)
		await expect(range).toHaveValue('all')
		await page.goto(`${base}/projects/${project.id}`)
		await expect(range).toHaveValue('custom')
		await expect(days).toHaveValue('3')
		await expandOwnProgress()
		await assertThreeDays()
		await range.selectOption('all')
		await assertAll()
		await page.reload()
		await expect(range).toHaveValue('all')
		await expandOwnProgress()
		await assertAll()
		await range.selectOption('custom')
		await days.fill('3')
		await days.press('Tab')
		await assertThreeDays()
		const bannerClose = page.getByRole('button', {name: 'Close banner', exact: true})
		if (await bannerClose.isVisible()) await bannerClose.click()
		await page.evaluate(() => window.scrollTo(0, 0))
		await page.screenshot({path: path.join(root!, 'progress-range-desktop.png'), fullPage: true, animations: 'disabled'})
		await page.setViewportSize({width: 390, height: 844})
		await expect(page.getByRole('button', {name: '显示菜单', exact: true})).toHaveAttribute('aria-expanded', 'false')
		await expect(range).toHaveValue('custom')
		await expect(days).toHaveValue('3')
		await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy()
		await page.evaluate(() => window.scrollTo(0, 0))
		await page.screenshot({path: path.join(root!, 'progress-range-mobile.png'), fullPage: true, animations: 'disabled'})
	} finally {
		for (const id of projectIds.reverse()) await request.delete(`${base}/api/v2/projects/${id}`, {headers}).catch(() => undefined)
	}
})
