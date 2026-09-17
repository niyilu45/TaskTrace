import {test, expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import path from 'node:path'
test('five-level hierarchy is enforced for new tasks and existing subtrees',async({page,request})=>{
 const root=process.env.TASKTRACE_LOCAL_TEST_DIR;test.skip(!root,'Requires isolated portable instance')
 const session=JSON.parse(readFileSync(path.join(root!,'data/local-session.json'),'utf8').replace(/^\uFEFF/,''))
 const base=readFileSync(path.join(root!,'data/local-config.yml'),'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
 const headers={Authorization:'Bearer '+session.token}
 async function create(url:string,data:object){const response=await request.post(base+'/api/v2'+url,{headers,data});expect(response.ok()).toBeTruthy();return response.json()}
 const project=await create('/projects',{title:'五级任务验收'})
 const chain=[]
 for(let i=1;i<=5;i++){
  const task=await create(`/projects/${project.id}/tasks`,{title:`第${i}级任务`});chain.push(task)
  if(i>1)await create(`/tasks/${chain[i-2].id}/relations`,{other_task_id:task.id,relation_kind:'subtask'})
 }
 const extra=await create(`/projects/${project.id}/tasks`,{title:'独立任务'})
 for(const [task,other,kind] of [[chain[4].id,extra.id,'subtask'],[extra.id,chain[4].id,'parenttask'],[extra.id,chain[0].id,'subtask'],[chain[0].id,extra.id,'parenttask']] as const){
  const rejected=await request.post(`${base}/api/v2/tasks/${task}/relations`,{headers,data:{other_task_id:other,relation_kind:kind}})
  expect(rejected.status()).toBe(409);expect((await rejected.json()).code).toBe(4090)
 }
 const branch=await create(`/projects/${project.id}/tasks`,{title:'附带子树的任务'})
 await create(`/tasks/${branch.id}/relations`,{other_task_id:extra.id,relation_kind:'subtask'})
 expect((await request.post(`${base}/api/v2/tasks/${chain[3].id}/relations`,{headers,data:{other_task_id:branch.id,relation_kind:'subtask'}})).status()).toBe(409)
 const unchanged=await(await request.get(`${base}/api/v2/tasks/${chain[4].id}`,{headers})).json()
 expect(unchanged.related_tasks?.subtask || []).toHaveLength(0)
 await page.goto(`${base}/tasks/${chain[4].id}#tasktrace-local=`+encodeURIComponent(JSON.stringify(session)))
 const relations=page.locator('.task-relations').last()
 await expect(relations).toContainText('任务最多支持 5 级')
 await expect(relations.getByRole('button',{name:'添加子任务',exact:true})).toBeDisabled()
 await expect(relations.getByRole('textbox',{name:'子任务名称',exact:true})).toBeDisabled()
 await page.goto(`${base}/tasks/${chain[3].id}`)
 await expect(relations).toContainText('当前第 4 级')
 await relations.getByRole('textbox',{name:'子任务名称',exact:true}).fill('另一个第5级任务')
 await relations.getByRole('button',{name:'添加子任务',exact:true}).click()
 await expect(relations.getByRole('status').filter({hasText:'子任务已添加'})).toBeVisible()
 await relations.getByRole('link',{name:/另一个第5级任务$/}).click()
 await expect(relations).toContainText('任务最多支持 5 级')
 await page.screenshot({path:path.join(root!,'depth-five.png'),fullPage:true})
})