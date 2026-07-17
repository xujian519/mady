#!/usr/bin/env python3
"""Mady P2A 全量 31 题本地基线跑批（一次性手动任务）。

连续运行 go test live 评估（本地 MLX 端点，免费），全程日志落盘；
生成/评判两阶段均按题缓存（$TMPDIR/mady_*.json[.judge]），被打断可续跑。
结果解析后写入 .eval-runs/p2a_result.json 并打印摘要。
"""

import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

REPO = Path("/Users/xujian/projects/Mady")
LOGDIR = REPO / ".eval-runs"
GO = "/opt/homebrew/bin/go"
TOTAL_TIMEOUT_S = 6900  # 单段上限 115 分钟；超时后下次运行续跑


def load_env() -> dict:
    env = dict(os.environ)
    for line in (REPO / ".env").read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            env[k.strip()] = v.strip()
    env.update({
        "MADY_LIVE_EVAL": "1",
        "MADY_EVAL_SUITE": "p2a",
        "MADY_EVAL_API_KEY": env.get("OMLX_API_KEY", ""),
        "MADY_EVAL_BASE_URL": "http://127.0.0.1:8000/v1",
        "MADY_EVAL_MODEL": "gemma-4-12B-it-8bit",
    })
    return env


def parse_results(text: str) -> dict:
    """从 go test -v 日志提取两个测试层级的通过率与各指标均值。"""
    out = {"tests": {}}
    current = None
    for line in text.splitlines():
        m = re.search(r"=== RUN\s+(Test\w+)", line)
        if m:
            current = m.group(1)
            out["tests"].setdefault(current, {})
        m = re.search(r"--- (PASS|FAIL|SKIP):\s+(Test\w+)", line)
        if m:
            out["tests"].setdefault(m.group(2), {})["status"] = m.group(1)
        if current:
            for key, pat in [
                ("total_cases", r"Total cases:\s+(\d+)"),
                ("passed_cases", r"Passed:\s+(\d+)"),
                ("pass_rate", r"Pass rate:\s+([\d.]+)"),
            ]:
                mm = re.search(pat, line)
                if mm:
                    out["tests"][current][key] = float(mm.group(1)) if "." in mm.group(1) else int(mm.group(1))
            mm = re.search(r"metric\s+(\w+)\s+mean:\s+([\d.]+)", line)
            if mm:
                out["tests"][current].setdefault("metrics", {})[mm.group(1)] = float(mm.group(2))
            mm = re.search(r"\[(PASS|FAIL)\]\s+(\S+)\s+avg=([\d.]+)\s+scores=(\{.*\})", line)
            if mm:
                out["tests"][current].setdefault("cases", []).append({
                    "status": mm.group(1), "case": mm.group(2),
                    "avg": float(mm.group(3)), "scores": mm.group(4),
                })
    out["log_tail"] = text.splitlines()[-5:]
    return out


def main() -> int:
    LOGDIR.mkdir(exist_ok=True)
    logpath = LOGDIR / "p2a_run.log"
    cmd = [
        GO, "test", "./agentcore/evaluate/benchmark/",
        "-run", "TestLiveDeepSeekEval$|TestLiveAgentBaselineEval$",
        "-count=1", "-timeout", "6800s", "-v",
    ]
    started = time.strftime("%Y-%m-%d %H:%M:%S")
    with logpath.open("w") as lf:
        proc = subprocess.run(
            cmd, cwd=REPO, env=load_env(),
            stdout=lf, stderr=subprocess.STDOUT,
            timeout=TOTAL_TIMEOUT_S,
        )
    text = logpath.read_text(errors="replace")
    result = parse_results(text)
    result.update({
        "started": started,
        "finished": time.strftime("%Y-%m-%d %H:%M:%S"),
        "exit_code": proc.returncode,
        "model": "gemma-4-12B-it-8bit",
        "endpoint": "local MLX 127.0.0.1:8000",
        "suite": "p2a (31 cases)",
    })
    (LOGDIR / "p2a_result.json").write_text(json.dumps(result, ensure_ascii=False, indent=2))
    summary = {k: {kk: vv for kk, vv in v.items() if kk != "cases"} for k, v in result["tests"].items()}
    # Daimon Automation 代码任务通过 stdout 最后一行的 AutomationOutput JSON 交付产物
    print(json.dumps({"artifact": {
        "tests": summary,
        "exit_code": proc.returncode,
        "model": result["model"],
        "suite": result["suite"],
        "log": str(logpath),
        "result_json": str(LOGDIR / "p2a_result.json"),
    }}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
