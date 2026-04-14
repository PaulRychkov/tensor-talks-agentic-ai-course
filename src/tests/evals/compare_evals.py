#!/usr/bin/env python3
"""Compare two eval runs and print diff (§10.4).

Usage:
  python tests/evals/compare_evals.py results/eval_A.json results/eval_B.json
"""

import json
import sys
from pathlib import Path


def load_result(path: str) -> dict:
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def compare_metric(name: str, val_a: float | None, val_b: float | None, threshold: float | None = None) -> str:
    if val_a is None or val_b is None:
        return f"  {name:<30} N/A"
    delta = val_b - val_a
    delta_str = f"{delta:+.3f}"
    threshold_str = ""
    if threshold is not None:
        ok = val_b >= threshold
        threshold_str = f" ({'✓' if ok else '❌'} threshold {threshold})"
    return f"  {name:<30} {val_a:.3f} → {val_b:.3f}  ({delta_str}){threshold_str}"


TRACKED_METRICS = {
    "decision_accuracy": 0.80,
    "score_accuracy": 0.70,
    "report_completeness": 1.0,
    "pass_rate": 0.70,
}


def main() -> int:
    if len(sys.argv) < 3:
        print("Usage: compare_evals.py <result_A.json> <result_B.json>")
        return 1

    data_a = load_result(sys.argv[1])
    data_b = load_result(sys.argv[2])

    print(f"\nComparing eval runs:")
    print(f"  A: {sys.argv[1]}")
    print(f"  B: {sys.argv[2]}")
    print()

    for agent_data_a in data_a.get("agents", []):
        agent = agent_data_a.get("agent", "?")
        agent_data_b = next(
            (x for x in data_b.get("agents", []) if x.get("agent") == agent), None
        )
        if not agent_data_b:
            print(f"[{agent}] Not found in B, skipping")
            continue

        metrics_a = agent_data_a.get("metrics", {})
        metrics_b = agent_data_b.get("metrics", {})

        print(f"[{agent.upper()}]")
        for metric, threshold in TRACKED_METRICS.items():
            print(compare_metric(metric, metrics_a.get(metric), metrics_b.get(metric), threshold))
        print()

    all_violations_b = []
    for agent_data in data_b.get("agents", []):
        all_violations_b.extend(agent_data.get("ci_violations", []))

    if all_violations_b:
        print("❌ CI gate violations in B:")
        for v in all_violations_b:
            print(f"   - {v}")
        return 1
    else:
        print("✓ All CI thresholds passed in B")
        return 0


if __name__ == "__main__":
    sys.exit(main())
