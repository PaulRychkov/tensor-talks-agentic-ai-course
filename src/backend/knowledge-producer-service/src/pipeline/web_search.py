"""Web search abstraction with allowlist/denylist and rate limiting (§9 p.5.9).

Provider is pluggable via WEB_SEARCH_PROVIDER config; keys via Vault.
"""

from __future__ import annotations

import re
import time
from typing import Any, Dict, List, Optional
from urllib.parse import urlparse

from ..config import settings
from ..logger.setup import get_logger

logger = get_logger(__name__)


def _parse_patterns(raw: str) -> List[str]:
    return [p.strip() for p in raw.split(",") if p.strip()]


TRUSTED_DOMAINS = _parse_patterns(settings.trusted_domain_patterns)
DENIED_DOMAINS = _parse_patterns(settings.denied_domain_patterns)


def is_url_allowed(url: str) -> bool:
    """Check URL against allow/deny lists."""
    host = urlparse(url).netloc.lower()
    if not host:
        return False
    for denied in DENIED_DOMAINS:
        if denied in host:
            return False
    if not TRUSTED_DOMAINS:
        return True
    return any(trusted in host for trusted in TRUSTED_DOMAINS)


class RateLimiter:
    """Simple token-bucket rate limiter per minute."""

    def __init__(self, max_per_minute: int):
        self._max = max_per_minute
        self._tokens: List[float] = []

    def acquire(self) -> bool:
        now = time.monotonic()
        self._tokens = [t for t in self._tokens if now - t < 60.0]
        if len(self._tokens) >= self._max:
            return False
        self._tokens.append(now)
        return True


class WebSearchProvider:
    """Abstract web search provider."""

    async def search(self, query: str, max_results: int = 5) -> List[Dict[str, Any]]:
        raise NotImplementedError


class NoopSearchProvider(WebSearchProvider):
    """Graceful degradation when no search provider configured."""

    async def search(self, query: str, max_results: int = 5) -> List[Dict[str, Any]]:
        logger.debug("No search provider configured, returning empty results")
        return []


class ArxivSearchProvider(WebSearchProvider):
    """Search arXiv API (free, no key required)."""

    API_URL = "http://export.arxiv.org/api/query"

    async def search(self, query: str, max_results: int = 5) -> List[Dict[str, Any]]:
        import httpx

        params = {
            "search_query": f"all:{query}",
            "start": 0,
            "max_results": max_results,
            "sortBy": "relevance",
        }
        async with httpx.AsyncClient(timeout=15) as client:
            resp = await client.get(self.API_URL, params=params)
            resp.raise_for_status()

        results = []
        entries = re.findall(r"<entry>(.*?)</entry>", resp.text, re.DOTALL)
        for entry in entries[:max_results]:
            title_m = re.search(r"<title>(.*?)</title>", entry, re.DOTALL)
            link_m = re.search(r'<id>(.*?)</id>', entry)
            summary_m = re.search(r"<summary>(.*?)</summary>", entry, re.DOTALL)
            results.append({
                "title": title_m.group(1).strip() if title_m else "",
                "url": link_m.group(1).strip() if link_m else "",
                "snippet": (summary_m.group(1).strip()[:300] if summary_m else ""),
                "source": "arxiv",
            })
        return results


class SemanticScholarProvider(WebSearchProvider):
    """Search Semantic Scholar Graph API (free tier)."""

    API_URL = "https://api.semanticscholar.org/graph/v1/paper/search"

    async def search(self, query: str, max_results: int = 5) -> List[Dict[str, Any]]:
        import httpx

        params = {"query": query, "limit": max_results, "fields": "title,url,abstract"}
        async with httpx.AsyncClient(timeout=15) as client:
            resp = await client.get(self.API_URL, params=params)
            resp.raise_for_status()
            data = resp.json()

        results = []
        for paper in data.get("data", [])[:max_results]:
            results.append({
                "title": paper.get("title", ""),
                "url": paper.get("url", ""),
                "snippet": (paper.get("abstract") or "")[:300],
                "source": "semantic_scholar",
            })
        return results


def create_search_provider() -> WebSearchProvider:
    """Factory based on WEB_SEARCH_PROVIDER config."""
    provider = settings.web_search_provider.lower()
    if provider == "arxiv":
        return ArxivSearchProvider()
    if provider in ("semantic_scholar", "semanticscholar"):
        return SemanticScholarProvider()
    return NoopSearchProvider()


class WebSearchService:
    """High-level search service with rate limiting and URL filtering."""

    def __init__(self):
        self._provider = create_search_provider()
        self._limiter = RateLimiter(settings.web_search_rate_limit_per_minute)
        self._calls_this_job = 0

    async def search(
        self,
        query: str,
        topic: Optional[str] = None,
        max_results: int = 5,
    ) -> List[Dict[str, Any]]:
        if self._calls_this_job >= settings.max_web_search_per_job:
            logger.warning("Web search limit per job reached")
            return []

        if not self._limiter.acquire():
            logger.warning("Web search rate limit hit")
            return []

        enriched_query = f"{topic} {query}" if topic else query
        self._calls_this_job += 1

        try:
            results = await self._provider.search(enriched_query, max_results)
            return [r for r in results if is_url_allowed(r.get("url", ""))]
        except Exception as e:
            logger.warning("Web search failed", error=str(e), provider=type(self._provider).__name__)
            return []

    def reset_job_counter(self) -> None:
        self._calls_this_job = 0
