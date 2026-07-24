// ego_lite_bridge.js
// 长生命周期进程，从 stdin 读取 JSON 命令，写入 JSON 结果到 stdout。
// 由 ego-browser nodejs 执行，ego-browser helper 全部预加载。

const readline = require('readline');
const rl = readline.createInterface({ input: process.stdin });

let currentTask = null;

rl.on('line', async (line) => {
  let req;
  try { req = JSON.parse(line); } catch (e) { return; }
  const { id, method, params } = req;
  try {
    const result = await dispatch(method, params || {});
    respond(id, true, result);
  } catch (e) {
    respond(id, false, null, e.message);
  }
});

rl.on('close', async () => {
  if (currentTask) {
    try { await completeTaskSpace(currentTask.id, { keep: true }); } catch {}
  }
  process.exit(0);
});
process.on('SIGTERM', () => rl.close());

function respond(id, ok, result, error) {
  const out = { id, ok };
  if (ok) { out.result = result; } else { out.error = error; }
  process.stdout.write(JSON.stringify(out) + '\n');
}

async function dispatch(method, params) {
  switch (method) {
    case 'ping':
      return 'pong';
    case 'initTaskSpace':
      currentTask = await useOrCreateTaskSpace(params.name);
      return { taskId: currentTask.taskId, id: currentTask.id };
    case 'navigate':
      await openOrReuseTab(params.url, { wait: true, timeout: params.timeout ?? 20 });
      return await snapshotText();
    case 'snapshotText':
      return await snapshotText(params);
    case 'captureScreenshot':
      const buf = await captureScreenshot();
      return buf.toString('base64');
    case 'click':
      await click(params.ref, params.label ? { label: params.label } : undefined);
      return await snapshotText();
    case 'typeText':
      await fillInput(params.ref, params.text);
      return `Typed "${params.text}" into ${params.ref}`;
    case 'scroll':
      await scrollBy(typeof params.dy === 'number' ? params.dy : 500);
      return await snapshotText();
    case 'pressKey':
      await pressKey(params.key);
      return `Pressed key: ${params.key}`;
    case 'evaluateJS':
      return await js(params.expression);
    case 'pageInfo':
      return await pageInfo();
    case 'handoffTaskSpace':
      return await handOffTaskSpace();
    case 'takeOverTaskSpace':
      await takeOverTaskSpace(currentTask ? currentTask.id : undefined);
      return {};
    case 'listTaskSpaces':
      const spaces = await listTaskSpaces();
      return spaces.map(s => ({ id: s.id, name: s.name, ownership: s.ownership }));
    case 'listTabs':
      const tabs = await listTabs();
      return tabs.map(t => ({ id: t.id, url: t.url, title: t.title }));
    case 'closeTab':
      await closeTab(params.targetId || undefined);
      return {};
    case 'completeTaskSpace':
      const res = await completeTaskSpace(currentTask ? currentTask.id : undefined, { keep: !!params.keep });
      currentTask = null;
      return res;
    default:
      throw new Error(`unknown method: ${method}`);
  }
}
