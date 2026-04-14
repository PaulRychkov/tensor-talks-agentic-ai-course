#!/usr/bin/env python3
"""
TensorTalks vs ChatGPT evaluation comparison (§10.4).

For each scenario in interviewer/scenarios.json that has an `expert_answer`
and `expert_score`, this script:
  1. Sends the question+candidate_answer to TT (via interviewer-agent API)
  2. Sends the same to GPT-4o as a judge (scoring prompt)
  3. Computes MAE and direction accuracy vs. expert_score

Usage:
  # Compare against the running TT agent (requires docker-compose up):
  python tests/evals/compare_tt_chatgpt.py \
      --tt-url http://localhost:8095 \
      --openai-key $OPENAI_API_KEY \
      --output tests/evals/results/comparison_$(date +%Y%m%d).json

  # Dry-run (no API calls, prints scenario list):
  python tests/evals/compare_tt_chatgpt.py --dry-run
"""

import argparse
import asyncio
import json
import os
import statistics
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

EVALS_DIR = Path(__file__).parent
SCENARIOS_PATH = EVALS_DIR / "interviewer" / "scenarios.json"

JUDGE_SYSTEM_PROMPT = """Ты — эксперт по ML и оценщик ответов кандидатов на технических интервью.
Оцени ответ кандидата на технический ML-вопрос по шкале от 0.0 до 1.0:
  0.0 — полностью неверный или отсутствует
  0.5 — частично правильный, основная идея есть, но детали отсутствуют
  1.0 — полный, точный, профессиональный ответ

Верни JSON: {"score": <float 0.0-1.0>, "reasoning": "<1-2 предложения>"}
Ничего кроме JSON."""

JUDGE_USER_TEMPLATE = """Вопрос: {question}

Ответ кандидата: {candidate_answer}

Эталонный ответ эксперта: {expert_answer}"""


def load_scenarios() -> List[Dict[str, Any]]:
    with open(SCENARIOS_PATH, encoding="utf-8") as f:
        scenarios = json.load(f)
    # Only keep scenarios with expert evaluation data
    return [s for s in scenarios if "expert_score" in s and "expert_answer" in s]


async def score_with_tt(
    session: Any, tt_url: str, scenario: Dict[str, Any]
) -> Optional[float]:
    """Call TT interviewer-agent evaluation endpoint and extract overall_score."""
    try:
        import aiohttp

        payload = {
            "question": scenario["question"],
            "user_answer": scenario["candidate_answer"],
            "theory": "",
            "session_mode": "interview",
        }
        async with session.post(
            f"{tt_url}/evaluate",
            json=payload,
            timeout=aiohttp.ClientTimeout(total=30),
        ) as resp:
            if resp.status != 200:
                print(f"  [TT] HTTP {resp.status} for {scenario['id']}", file=sys.stderr)
                return None
            data = await resp.json()
            return data.get("overall_score") or data.get("score")
    except Exception as exc:
        print(f"  [TT] Error for {scenario['id']}: {exc}", file=sys.stderr)
        return None


async def score_with_gpt(
    openai_key: str, scenario: Dict[str, Any]
) -> Optional[float]:
    """Use GPT-4o as a judge to score the candidate answer."""
    try:
        import aiohttp

        headers = {
            "Authorization": f"Bearer {openai_key}",
            "Content-Type": "application/json",
        }
        body = {
            "model": "gpt-4o",
            "messages": [
                {"role": "system", "content": JUDGE_SYSTEM_PROMPT},
                {
                    "role": "user",
                    "content": JUDGE_USER_TEMPLATE.format(
                        question=scenario["question"],
                        candidate_answer=scenario["candidate_answer"],
                        expert_answer=scenario["expert_answer"],
                    ),
                },
            ],
            "temperature": 0,
            "response_format": {"type": "json_object"},
        }
        async with aiohttp.ClientSession() as sess:
            async with sess.post(
                "https://api.openai.com/v1/chat/completions",
                headers=headers,
                json=body,
                timeout=aiohttp.ClientTimeout(total=30),
            ) as resp:
                if resp.status != 200:
                    print(f"  [GPT] HTTP {resp.status} for {scenario['id']}", file=sys.stderr)
                    return None
                data = await resp.json()
                content = data["choices"][0]["message"]["content"]
                parsed = json.loads(content)
                return float(parsed["score"])
    except Exception as exc:
        print(f"  [GPT] Error for {scenario['id']}: {exc}", file=sys.stderr)
        return None


def compute_metrics(
    results: List[Dict[str, Any]], system: str
) -> Dict[str, float]:
    """Compute MAE and direction accuracy vs. expert scores."""
    scores = [(r["expert_score"], r.get(f"{system}_score")) for r in results]
    valid = [(exp, pred) for exp, pred in scores if pred is not None]

    if not valid:
        return {"mae": -1, "direction_accuracy": -1, "n": 0}

    mae = statistics.mean(abs(exp - pred) for exp, pred in valid)

    # Direction accuracy: does the system rank high/low the same as expert?
    # Expert threshold: 0.6 = pass, below = needs_work
    direction_correct = sum(
        1 for exp, pred in valid if (exp >= 0.6) == (pred >= 0.6)
    )
    direction_acc = direction_correct / len(valid)

    return {
        "mae": round(mae, 4),
        "direction_accuracy": round(direction_acc, 4),
        "n": len(valid),
        "n_total": len(scores),
    }


async def run_comparison(
    tt_url: Optional[str],
    openai_key: Optional[str],
    dry_run: bool,
    output_path: Optional[str],
):
    scenarios = load_scenarios()
    print(f"Loaded {len(scenarios)} scenarios with expert scores")

    if dry_run:
        for s in scenarios:
            print(f"  {s['id']} [{s['topic']}] expert={s['expert_score']:.2f} — {s['question'][:60]}…")
        return

    results = []

    try:
        import aiohttp
    except ImportError:
        print("ERROR: aiohttp required — pip install aiohttp", file=sys.stderr)
        sys.exit(1)

    async with aiohttp.ClientSession() as session:
        for scenario in scenarios:
            print(f"  Evaluating {scenario['id']} [{scenario['category']}]…")
            result: Dict[str, Any] = {
                "id": scenario["id"],
                "topic": scenario["topic"],
                "category": scenario["category"],
                "question": scenario["question"],
                "candidate_answer": scenario["candidate_answer"],
                "expert_score": scenario["expert_score"],
            }

            if tt_url:
                tt_score = await score_with_tt(session, tt_url, scenario)
                result["tt_score"] = tt_score
                print(f"    TT: {tt_score}")

            if openai_key:
                gpt_score = await score_with_gpt(openai_key, scenario)
                result["gpt_score"] = gpt_score
                print(f"    GPT: {gpt_score}")

            print(f"    Expert: {scenario['expert_score']}")
            results.append(result)
            await asyncio.sleep(0.5)  # Rate limit

    print("\n=== Summary ===")
    if tt_url:
        m = compute_metrics(results, "tt")
        print(f"TensorTalks  — MAE: {m['mae']:.4f}, Direction: {m['direction_accuracy']:.2%} ({m['n']}/{m['n_total']})")

    if openai_key:
        m = compute_metrics(results, "gpt")
        print(f"ChatGPT (4o) — MAE: {m['mae']:.4f}, Direction: {m['direction_accuracy']:.2%} ({m['n']}/{m['n_total']})")

    output = {
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "scenarios_count": len(scenarios),
        "results": results,
        "metrics": {
            "tt": compute_metrics(results, "tt") if tt_url else None,
            "gpt": compute_metrics(results, "gpt") if openai_key else None,
        },
    }

    if output_path:
        Path(output_path).parent.mkdir(parents=True, exist_ok=True)
        with open(output_path, "w", encoding="utf-8") as f:
            json.dump(output, f, ensure_ascii=False, indent=2)
        print(f"\nResults saved to {output_path}")
    else:
        print("\nResults:")
        print(json.dumps(output, ensure_ascii=False, indent=2))


def main():
    parser = argparse.ArgumentParser(description="TT vs ChatGPT eval comparison")
    parser.add_argument(
        "--tt-url",
        default=os.environ.get("TT_AGENT_URL", "http://localhost:8095"),
        help="TT interviewer-agent base URL",
    )
    parser.add_argument(
        "--openai-key",
        default=os.environ.get("OPENAI_API_KEY"),
        help="OpenAI API key for GPT judge",
    )
    parser.add_argument(
        "--output",
        default=None,
        help="Output JSON file path",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="List scenarios without calling APIs",
    )
    parser.add_argument(
        "--no-tt",
        action="store_true",
        help="Skip TT evaluation (GPT only)",
    )
    args = parser.parse_args()

    tt_url = None if args.no_tt else args.tt_url
    openai_key = args.openai_key

    if not args.dry_run and not tt_url and not openai_key:
        print("ERROR: Provide --tt-url and/or --openai-key", file=sys.stderr)
        sys.exit(1)

    asyncio.run(run_comparison(tt_url, openai_key, args.dry_run, args.output))


if __name__ == "__main__":
    main()
