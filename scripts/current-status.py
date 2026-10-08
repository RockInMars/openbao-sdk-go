#!/usr/bin/env python3
"""Derive a current summary from existing ledgers and receipts; never edit them."""
import argparse
from collections import Counter
import hashlib
import pathlib
import sys

from evidence import (FORMAL_CLASSES, FORMAL_REPORT_NAMES, identity_matches, read_json,
                      release_problems, report_results, safe_file)
from tooling import ROOT, source_hash


def load(root, path):
    try:
        value = read_json(safe_file(root, path))
        return value if isinstance(value, dict) else {}
    except (ValueError, OSError, TypeError):
        return {}


def record_counts(record, field):
    values = record.get(field, [])
    if not isinstance(values, list):
        return 'INVALID'
    allowed = {'PASS', 'FAIL', 'PARTIAL', 'BLOCKED', 'NOT_RUN', 'VERIFIED', 'PENDING', 'IN_PROGRESS'}
    statuses = [item.get('status') if isinstance(item, dict) else None for item in values]
    counts = Counter(value if isinstance(value, str) and value in allowed else 'INVALID' for value in statuses)
    return ' / '.join(f'{key} {value}' for key, value in sorted(counts.items())) or '无记录'


def index_state(root, ledger, current_hash):
    index = load(root, ledger.get('evidence_index'))
    entries = index.get('reports', [])
    if not isinstance(entries, list) or not entries:
        return '缺失或无报告条目'
    seen = set()
    valid = 0
    for item in entries:
        try:
            name = item['path']
            raw = safe_file(root, name).read_bytes()
            if name in seen or hashlib.sha256(raw).hexdigest() != item.get('sha256') or len(raw) != item.get('bytes'):
                continue
            seen.add(name)
            valid += 1
        except (ValueError, OSError, KeyError, TypeError):
            continue
    baseline = 'CURRENT' if identity_matches(index, current_hash) else 'STALE'
    return f'{baseline}；文件摘要匹配 {valid}/{len(entries)}'


def summarize(root=ROOT, *, current_hash=None):
    root = pathlib.Path(root)
    current_hash = source_hash(root) if current_hash is None else current_hash
    ledger = load(root, 'task-status.json')
    acceptance = load(root, 'acceptance-results.json')
    reports = {name: load(root, '.artifacts/' + file + '-report.json') for name, file in FORMAL_REPORT_NAMES.items()}
    rows = []
    identity = {'source_hash_version': 2, 'source_sha256': current_hash}
    for name, kind in FORMAL_CLASSES.items():
        report = reports[name]
        recorded = report.get('status')
        if recorded not in {'PASS', 'FAIL', 'BLOCKED', 'NOT_RUN'}:
            recorded = '无有效记录'
        if not report:
            state = 'NOT_RUN'
        elif not identity_matches(report, current_hash) or report.get('source_before') != identity or report.get('source_after') != identity:
            state = 'STALE'
        elif report.get('evidence_class') != kind:
            state = 'INVALID'
        elif recorded == 'PASS':
            try:
                if name.startswith('minimum-') and report.get('platform') != name.rsplit('-', 1)[1]:
                    raise ValueError('platform mismatch')
                report_results(root, report, current_hash)
                state = 'PASS'
            except (ValueError, OSError, KeyError, TypeError, AttributeError):
                state = 'INVALID'
        else:
            state = recorded if recorded in {'FAIL', 'BLOCKED', 'NOT_RUN'} else 'INVALID'
        rows.append({'name': name, 'class': kind, 'recorded': recorded, 'state': state,
                     'path': '.artifacts/' + FORMAL_REPORT_NAMES[name] + '-report.json'})
    try:
        problems = release_problems(root, ledger, acceptance, reports, current_hash)
    except (ValueError, OSError, KeyError, TypeError, AttributeError):
        problems = ['invalid release inputs']
    remote = acceptance.get('supplemental_remote', {})
    remote = remote if isinstance(remote, dict) else {}
    remote_status = remote.get('status') if remote.get('status') in {'PASS', 'FAIL', 'BLOCKED', 'NOT_RUN'} else '无有效记录'
    return {'source_sha256': current_hash, 'rows': rows,
            'ledger_current': identity_matches(ledger, current_hash),
            'acceptance_current': identity_matches(acceptance, current_hash),
            'task_counts': record_counts(ledger, 'tasks'), 'acceptance_counts': record_counts(acceptance, 'results'),
            'index': index_state(root, ledger, current_hash), 'gate': 'FAIL' if problems else 'PASS',
            'gate_refusals': len(problems), 'remote_status': remote_status,
            'remote_current': identity_matches(remote, current_hash) and remote.get('historical_only') is False}


def render(summary):
    current = lambda value: 'CURRENT' if value else 'STALE'
    lines = ['# 当前验证状态（自动生成）', '',
             '由 `scripts/current-status.py` 读取现有台账、证据索引和报告生成；此页不修改验收状态，也不授予发布批准。', '',
             f'源码标识：v2 `{summary["source_sha256"]}`。', '',
             f'- 任务台账：{current(summary["ledger_current"])}；记录状态为 {summary["task_counts"]}。',
             f'- 验收台账：{current(summary["acceptance_current"])}；记录状态为 {summary["acceptance_counts"]}。',
             f'- 台账指向的证据索引：{summary["index"]}。',
             f'- 按当前源码即时计算的发布门禁：**{summary["gate"]}**，{summary["gate_refusals"]} 项拒绝。', '',
             '| 报告 | 证据类别 | 报告原状态 | 当前校验 |', '| --- | --- | --- | --- |']
    for row in summary['rows']:
        lines.append(f'| [{row["name"]}](../{row["path"]}) | `{row["class"]}` | {row["recorded"]} | {row["state"]} |')
    lines += ['', 'PASS 要求当前源码、前后标识、实际目标、命令、工具链、平台及日志摘要通过现有证据校验。STALE 表示历史源码；INVALID 表示当前报告不能支持其声明；NOT_RUN 包括缺失报告。BLOCKED/FAIL 保留报告原有结论，不代表检查成功。', '',
              f'补充远端：台账记录 {summary["remote_status"]}，归属 {current(summary["remote_current"])}；它始终独立于 `REAL_OPENBAO`，不替代正式 fixture、扫描或最低版本验证。此处不重新执行远端检查。', '',
              '重生成：`python -B scripts/current-status.py --write`；一致性检查：`python -B scripts/current-status.py --check`。本地命令遵循全局 RTK 规则。', '',
              '具体范围、首次失败、审查和续作见[实施交接](implementation-handoff.md)；原始事实见[任务台账](../task-status.json)、[验收台账](../acceptance-results.json)。', '']
    return '\n'.join(lines)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument('--write', action='store_true')
    action.add_argument('--check', action='store_true')
    args = parser.parse_args(argv)
    target = ROOT / 'docs/current-status.md'
    expected = render(summarize())
    if args.write:
        target.write_text(expected, encoding='utf-8', newline='\n')
        print('Current summary written: docs/current-status.md')
        return 0
    if target.is_file() and target.read_text(encoding='utf-8') == expected:
        print('CURRENT SUMMARY: PASS (freshness only; not release approval)')
        return 0
    print('CURRENT SUMMARY: FAIL; regenerate from current ledgers and receipts')
    return 1


if __name__ == '__main__':
    sys.exit(main())
