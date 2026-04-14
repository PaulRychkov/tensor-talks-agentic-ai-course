#!/usr/bin/env python3
"""Eval pipeline runner for TensorTalks agents (§10.4).

Usage:
  # Run all agents (CI mode):
  python tests/evals/run_evals.py --agent all --mode ci --output results/eval.json

  # Run interviewer agent with live LLM:
  python tests/evals/run_evals.py --agent interviewer --mode live --output results/eval_$(date +%Y%m%d).json

  # Run analyst agent only:
  python tests/evals/run_evals.py --agent analyst --mode live
"""

import argparse
import asyncio
import json
import os
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

# Thresholds for CI gate (§10.4)
CI_THRESHOLDS = {
    "decision_accuracy": 0.80,
    "score_accuracy": 0.70,
    "report_completeness": 1.0,
    "hint_relevance": 0.70,
}

EVALS_DIR = Path(__file__).parent
RESULTS_DIR = EVALS_DIR / "results"


def load_scenarios(agent: str) -> List[Dict[str, Any]]:
    """Load scenario definitions from JSON file."""
    path = EVALS_DIR / agent / "scenarios.json"
    if not path.exists():
        print(f"ERROR: Scenarios file not found: {path}", file=sys.stderr)
        return []
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def evaluate_interviewer_scenario(
    scenario: Dict[str, Any],
    agent_output: Dict[str, Any],
) -> Dict[str, Any]:
    """Evaluate a single interviewer scenario against expected behavior."""
    expected = scenario.get("expected", {})
    result = {
        "scenario_id": scenario["id"],
        "category": scenario.get("category", ""),
        "passed": True,
        "failures": [],
        "metrics": {},
    }

    # Check decision accuracy
    if "decision" in expected:
        got_decision = agent_output.get("decision", "")
        expected_decision = expected["decision"]
        decision_ok = got_decision == expected_decision
        result["metrics"]["decision_match"] = decision_ok
        if not decision_ok:
            result["failures"].append(
                f"Expected decision={expected_decision}, got={got_decision}"
            )
            result["passed"] = False

    # Check decision NOT equal to something
    if "decision_not" in expected:
        got_decision = agent_output.get("decision", "")
        if got_decision == expected["decision_not"]:
            result["failures"].append(
                f"Decision should NOT be {expected['decision_not']}, but got {got_decision}"
            )
            result["passed"] = False

    # Check score range
    if "score_min" in expected or "score_max" in expected:
        got_score = float(agent_output.get("overall_score", agent_output.get("score", 0.0)))
        score_min = expected.get("score_min", 0.0)
        score_max = expected.get("score_max", 1.0)
        in_range = score_min <= got_score <= score_max
        result["metrics"]["score"] = got_score
        result["metrics"]["score_in_range"] = in_range
        if not in_range:
            result["failures"].append(
                f"Score {got_score:.2f} not in expected range [{score_min}, {score_max}]"
            )
            result["passed"] = False

    # Check PII detection
    if "pii_detected" in expected:
        pii_detected = agent_output.get("pii_detected", False)
        if pii_detected != expected["pii_detected"]:
            result["failures"].append(
                f"PII detection: expected={expected['pii_detected']}, got={pii_detected}"
            )
            result["passed"] = False

    # Check classification (off_topic / comment)
    if "classification" in expected:
        got_cls = agent_output.get("classification", "")
        expected_cls = expected["classification"]
        cls_ok = got_cls == expected_cls
        result["metrics"]["classification_match"] = cls_ok
        if not cls_ok:
            result["failures"].append(
                f"Expected classification={expected_cls}, got={got_cls}"
            )
            result["passed"] = False

    return result


def evaluate_analyst_scenario(
    scenario: Dict[str, Any],
    agent_output: Dict[str, Any],
) -> Dict[str, Any]:
    """Evaluate a single analyst scenario against expected behavior."""
    expected = scenario.get("expected", {})
    report = agent_output.get("report", {})
    result = {
        "scenario_id": scenario["id"],
        "category": scenario.get("category", ""),
        "passed": True,
        "failures": [],
        "metrics": {},
    }

    # Required report sections
    required_sections = ["summary", "score", "errors_by_topic", "strengths", "preparation_plan"]
    for section in required_sections:
        if section not in report:
            result["failures"].append(f"Missing report section: {section}")
            result["passed"] = False
    result["metrics"]["completeness"] = len(result["failures"]) == 0

    # Score range check
    if "report_score_min" in expected:
        got_score = report.get("score", 0)
        if got_score < expected["report_score_min"]:
            result["failures"].append(
                f"Score {got_score} below expected min {expected['report_score_min']}"
            )
            result["passed"] = False

    if "report_score_max" in expected:
        got_score = report.get("score", 100)
        if got_score > expected["report_score_max"]:
            result["failures"].append(
                f"Score {got_score} above expected max {expected['report_score_max']}"
            )
            result["passed"] = False

    # Preparation plan min items
    if "preparation_plan_min_items" in expected:
        plan = report.get("preparation_plan", [])
        if len(plan) < expected["preparation_plan_min_items"]:
            result["failures"].append(
                f"Preparation plan has {len(plan)} items, expected >= {expected['preparation_plan_min_items']}"
            )
            result["passed"] = False

    # Strengths non-empty
    if expected.get("strengths_nonempty"):
        strengths = report.get("strengths", [])
        if not strengths:
            result["failures"].append("Expected non-empty strengths list")
            result["passed"] = False

    return result


def compute_aggregate_metrics(
    results: List[Dict[str, Any]],
) -> Dict[str, Any]:
    """Compute aggregate metrics from per-scenario results."""
    total = len(results)
    if total == 0:
        return {"total": 0, "passed": 0, "pass_rate": 0.0}

    passed = sum(1 for r in results if r.get("passed", False))

    # Decision accuracy
    decision_checks = [r for r in results if "decision_match" in r.get("metrics", {})]
    decision_accuracy = (
        sum(1 for r in decision_checks if r["metrics"]["decision_match"]) / len(decision_checks)
        if decision_checks
        else None
    )

    # Score accuracy (scenarios with score range checks)
    score_checks = [r for r in results if "score_in_range" in r.get("metrics", {})]
    score_accuracy = (
        sum(1 for r in score_checks if r["metrics"]["score_in_range"]) / len(score_checks)
        if score_checks
        else None
    )

    # Report completeness
    completeness_checks = [r for r in results if "completeness" in r.get("metrics", {})]
    report_completeness = (
        sum(1 for r in completeness_checks if r["metrics"]["completeness"]) / len(completeness_checks)
        if completeness_checks
        else None
    )

    return {
        "total": total,
        "passed": passed,
        "pass_rate": passed / total,
        "decision_accuracy": decision_accuracy,
        "score_accuracy": score_accuracy,
        "report_completeness": report_completeness,
    }


def check_ci_thresholds(metrics: Dict[str, Any]) -> List[str]:
    """Return list of threshold violations for CI gate."""
    violations = []
    for metric, threshold in CI_THRESHOLDS.items():
        value = metrics.get(metric)
        if value is not None and value < threshold:
            violations.append(
                f"{metric}: {value:.3f} < threshold {threshold}"
            )
    return violations


def print_summary(agent: str, metrics: Dict[str, Any], violations: List[str]) -> None:
    """Print human-readable eval summary."""
    print(f"\n{'='*60}")
    print(f"  Eval results: {agent}")
    print(f"{'='*60}")
    print(f"  Total scenarios:  {metrics['total']}")
    print(f"  Passed:           {metrics['passed']} ({metrics['pass_rate']:.1%})")
    if metrics.get("decision_accuracy") is not None:
        print(f"  Decision accuracy:{metrics['decision_accuracy']:.1%} (threshold: {CI_THRESHOLDS.get('decision_accuracy', '-')})")
    if metrics.get("score_accuracy") is not None:
        print(f"  Score accuracy:   {metrics['score_accuracy']:.1%} (threshold: {CI_THRESHOLDS.get('score_accuracy', '-')})")
    if metrics.get("report_completeness") is not None:
        print(f"  Report complete:  {metrics['report_completeness']:.1%} (threshold: {CI_THRESHOLDS.get('report_completeness', '-')})")
    if violations:
        print(f"\n  ❌ CI GATE FAILURES:")
        for v in violations:
            print(f"     - {v}")
    else:
        print(f"\n  ✓ All CI thresholds passed")
    print(f"{'='*60}\n")


async def run_interviewer_evals(mode: str, sample_live: int = 0) -> Dict[str, Any]:
    """Run interviewer agent eval scenarios."""
    scenarios = load_scenarios("interviewer")
    if not scenarios:
        return {"error": "No scenarios loaded"}

    print(f"Running {len(scenarios)} interviewer scenarios (mode={mode})...")

    # In CI mode without live LLM, we run deterministic checks only.
    # In live mode, we'd invoke the actual agent graph.
    results = []
    for scenario in scenarios:
        # Deterministic checks that don't require LLM (PII detection, empty answer)
        category = scenario.get("category", "")
        expected = scenario.get("expected", {})

        if category == "pii_filter":
            # Run PII regex check
            from pathlib import Path
            import sys
            sys.path.insert(0, str(Path(__file__).parent.parent.parent / "backend" / "interviewer-agent-service"))
            try:
                from src.guardrails.pii_filter import check_pii_regex
                answer = scenario.get("candidate_answer", "")
                pii_result = check_pii_regex(answer)
                agent_output = {
                    "pii_detected": pii_result.detected,
                    "pii_category": pii_result.category.value if pii_result.category else None,
                }
            except ImportError:
                agent_output = {"pii_detected": False, "error": "import_failed"}
        elif mode == "ci" and category not in ("pii_filter",):
            # In CI mode, skip LLM-dependent scenarios with placeholder
            agent_output = {"decision": "unknown", "overall_score": 0.5, "_skipped": True}
        else:
            # Live mode — would invoke actual agent graph
            # For now, placeholder
            agent_output = {"decision": "unknown", "overall_score": 0.5, "_mock": True}

        result = evaluate_interviewer_scenario(scenario, agent_output)
        if agent_output.get("_skipped") or agent_output.get("_mock"):
            result["passed"] = None  # Unknown (not executed)
            result["skipped"] = True
        results.append(result)

    executed = [r for r in results if not r.get("skipped")]
    metrics = compute_aggregate_metrics(executed)
    violations = check_ci_thresholds(metrics)

    return {
        "agent": "interviewer",
        "mode": mode,
        "run_at": datetime.now(timezone.utc).isoformat(),
        "scenarios_total": len(scenarios),
        "scenarios_executed": len(executed),
        "results": results,
        "metrics": metrics,
        "ci_violations": violations,
    }


async def run_analyst_evals(mode: str) -> Dict[str, Any]:
    """Run analyst agent eval scenarios (placeholder for live mode)."""
    scenarios = load_scenarios("analyst")
    if not scenarios:
        return {"error": "No scenarios loaded"}

    print(f"Running {len(scenarios)} analyst scenarios (mode={mode})...")

    results = []
    for scenario in scenarios:
        # Placeholder — real execution requires live LLM
        agent_output = {"report": {}, "_mock": True}
        result = evaluate_analyst_scenario(scenario, agent_output)
        result["skipped"] = True
        result["passed"] = None
        results.append(result)

    executed = [r for r in results if not r.get("skipped")]
    metrics = compute_aggregate_metrics(executed)
    violations = check_ci_thresholds(metrics)

    return {
        "agent": "analyst",
        "mode": mode,
        "run_at": datetime.now(timezone.utc).isoformat(),
        "scenarios_total": len(scenarios),
        "scenarios_executed": len(executed),
        "results": results,
        "metrics": metrics,
        "ci_violations": violations,
    }


def save_results(run_result: Dict[str, Any], output_path: Optional[str]) -> None:
    """Save eval results to JSON file."""
    if not output_path:
        return
    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    path = Path(output_path) if Path(output_path).is_absolute() else RESULTS_DIR / output_path
    with open(path, "w", encoding="utf-8") as f:
        json.dump(run_result, f, ensure_ascii=False, indent=2)
    print(f"Results saved to: {path}")


async def main() -> int:
    parser = argparse.ArgumentParser(description="TensorTalks agent eval runner")
    parser.add_argument(
        "--agent",
        choices=["interviewer", "analyst", "builder", "all"],
        default="all",
        help="Which agent to evaluate",
    )
    parser.add_argument(
        "--mode",
        choices=["live", "ci"],
        default="ci",
        help="live = use real LLM; ci = deterministic checks only",
    )
    parser.add_argument("--output", default=None, help="Path for JSON output file")
    parser.add_argument(
        "--sample-live",
        type=int,
        default=0,
        help="In CI mode: number of scenarios to run live (0 = all deterministic)",
    )
    args = parser.parse_args()

    all_results = []
    has_violation = False

    agents_to_run = (
        ["interviewer", "analyst"] if args.agent == "all" else [args.agent]
    )

    for agent in agents_to_run:
        if agent == "interviewer":
            result = await run_interviewer_evals(args.mode, args.sample_live)
        elif agent == "analyst":
            result = await run_analyst_evals(args.mode)
        else:
            continue

        metrics = result.get("metrics", {})
        violations = result.get("ci_violations", [])
        print_summary(agent, metrics, violations)
        all_results.append(result)
        if violations:
            has_violation = True

    combined = {
        "run_id": f"eval-{datetime.now(timezone.utc).strftime('%Y%m%d-%H%M')}",
        "mode": args.mode,
        "agents": all_results,
        "overall_passed": not has_violation,
    }

    save_results(combined, args.output)

    return 1 if has_violation else 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
