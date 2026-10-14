#!/usr/bin/env python3
"""Maintain the local Guanlan task ledger and its human-readable checklist."""
import argparse
import datetime
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PLAN = ROOT / 'docs/plans/guanlan'

def render(data):
    status_line = ('状态：**30项任务已完成并通过必要验收。**' if data['status'] == 'complete' else
        '状态：**已获用户开工授权，按任务依赖持续实施。**')
    lines = ['# 观澜与看板组件：长任务清单', '',
        status_line + ' 先组件库、我的看板与编辑配置，后观澜展示。', '',
        '30项任务保留原ID；[方案](README.md)、[任务账本](tasks.json)、[接续状态](STATE.md)。', '',
        '| 阶段 | 任务 | 阶段成果 |', '| --- | --- | --- |']
    for phase in data['phases']:
        lines.append(f"| {phase['id']} {phase['title']} | {phase['range']} | {phase['outcome']} |")
    for phase in data['phases']:
        lines.extend(['', f"## {phase['id']} {phase['title']}", ''])
        for task in data['tasks']:
            if task['phase'] != phase['id']:
                continue
            mark = 'x' if task['status'] == 'done' else ' '
            deps = '、'.join(task['depends_on']) or '无'
            lines.extend([f"- [{mark}] **{task['id']} {task['title']}**", '',
                f"  状态：`{task['status']}`；依赖：{deps}。", '', '  交付：', ''])
            lines.extend('  - ' + v for v in task['deliverables'])
            lines.extend(['', '  验收：', ''])
            lines.extend('  - ' + v for v in task['acceptance'])
            lines.extend(['', '  进展：' + task['progress_note'],
                '  证据：' + ('、'.join(task['evidence']) or '待实施验证后填写'), ''])
    lines.extend(['## 执行规则', '',
        '每项完成更新账本、清单和STATE；每完成重要阶段反馈。done须有交付及必要验证证据。布局完成不解除ES5实盘验收缺口。普通阶段无需再次确认，不自动调用真实AI、提交推送或发布。', ''])
    (PLAN / 'TASKS.md').write_text('\n'.join(lines), encoding='utf-8')

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--start', action='store_true')
    parser.add_argument('--task')
    parser.add_argument('--status', choices=['todo', 'in_progress', 'blocked', 'done'])
    parser.add_argument('--note')
    parser.add_argument('--evidence', action='append', default=[])
    args = parser.parse_args()
    data = json.loads((PLAN / 'tasks.json').read_text(encoding='utf-8'))
    tasks = {t['id']: t for t in data['tasks']}
    if args.start:
        data['authorization']['implementation_authorized'] = True
        data['authorization']['start_instruction'] = '用户明确指令：开工'
        data['authorization']['execution_order_instruction'] = '先组件库、我的看板和编辑配置，后观澜展示'
        data['status'] = 'in_progress'
        tasks['GL-008']['depends_on'] = ['GL-002', 'GL-006']
        tasks['GL-013']['depends_on'] = ['GL-003', 'GL-006', 'GL-008']
        tasks['GL-018']['depends_on'] = ['GL-005', 'GL-017']
        tasks['GL-021']['depends_on'] = ['GL-018', 'GL-019', 'GL-005']
        tasks['GL-007']['depends_on'] = ['GL-021', 'GL-025']
        tasks['GL-026']['depends_on'] = ['GL-021', 'GL-025', 'GL-020']
        groups = [[1, 2, 3], [4, 5, 6], list(range(8, 13)), list(range(13, 18)),
            [18, 19, 21], [22, 23, 24, 25, 7, 20], list(range(26, 31))]
        titles = ['保护基线与实现合同', '配置版本与组件存储', '八类组件与数据能力',
            '布局编辑器', '我的看板与配置历史', '组件库与观澜联动', '整体验收与交付']
        outcomes = ['备份可恢复、合同固定、布局方案验证。', '草稿、版本、启用指针与预设独立存储。',
            '先完成八类组件和共享数据适配。', '拖入、移动缩放、样式设置与保存链路可用。',
            '两条/页，新建复制、预览及归档恢复。', '组件库/预设齐备后接通观澜与显式启用。',
            'Web/桌面实际验收，数据保护与效率核对，完成交付。']
        order = []
        for i, numbers in enumerate(groups):
            ids = [f'GL-{n:03}' for n in numbers]
            phase = data['phases'][i]
            phase.update(title=titles[i], range='、'.join(ids), outcome=outcomes[i])
            for ident in ids:
                tasks[ident]['phase'] = phase['id']
            order.extend(ids)
        data['execution_order'] = order
        data['tasks'] = [tasks[ident] for ident in order]
    if args.task:
        task = tasks[args.task]
        if args.status:
            if args.status == 'done' and not (task['evidence'] or args.evidence):
                raise SystemExit('done requires evidence')
            task['status'] = args.status
        if args.note:
            task['progress_note'] = args.note
        task['evidence'] = list(dict.fromkeys(task['evidence'] + args.evidence))
    seen = set()
    for task in data['tasks']:
        if not set(task['depends_on']) <= seen:
            raise SystemExit('invalid dependency order: ' + task['id'])
        seen.add(task['id'])
    data['next_task'] = next((t['id'] for t in data['tasks'] if t['status'] != 'done'), None)
    data['updated_at'] = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat()
    (PLAN / 'tasks.json').write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    render(data)
    print(json.dumps({'status': data['status'], 'next_task': data['next_task'],
        'done': sum(t['status'] == 'done' for t in data['tasks'])}, ensure_ascii=False))

if __name__ == '__main__':
    main()
