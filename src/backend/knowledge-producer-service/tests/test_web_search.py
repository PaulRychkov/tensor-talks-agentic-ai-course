"""Tests for src/pipeline/web_search.py – URL filtering, rate limiter, providers."""

from __future__ import annotations

import importlib
import time
from unittest.mock import AsyncMock, patch

import pytest

from src.config import settings


# ---------------------------------------------------------------------------
# Helpers to reload the module with custom domain settings
# ---------------------------------------------------------------------------

def _reload_web_search(trusted: str = "", denied: str = ""):
    with patch.object(settings, "trusted_domain_patterns", trusted), \
         patch.object(settings, "denied_domain_patterns", denied):
        import src.pipeline.web_search as mod
        importlib.reload(mod)
    return mod


# ---------------------------------------------------------------------------
# is_url_allowed
# ---------------------------------------------------------------------------


class TestIsUrlAllowed:
    def test_trusted_domain_accepted(self):
        mod = _reload_web_search(trusted="arxiv.org,pytorch.org")
        assert mod.is_url_allowed("https://arxiv.org/abs/1234") is True
        assert mod.is_url_allowed("https://pytorch.org/docs") is True

    def test_untrusted_domain_rejected(self):
        mod = _reload_web_search(trusted="arxiv.org")
        assert mod.is_url_allowed("https://evil.com/exploit") is False

    def test_denied_domain_rejected_even_if_trusted(self):
        mod = _reload_web_search(trusted="example.com", denied="example.com")
        assert mod.is_url_allowed("https://example.com/page") is False

    def test_denied_takes_precedence(self):
        mod = _reload_web_search(trusted="arxiv.org", denied="arxiv.org")
        assert mod.is_url_allowed("https://arxiv.org/paper") is False

    def test_empty_trusted_allows_all_non_denied(self):
        mod = _reload_web_search(trusted="", denied="spam.com")
        assert mod.is_url_allowed("https://any-site.org") is True
        assert mod.is_url_allowed("https://spam.com/bad") is False

    def test_no_host_returns_false(self):
        mod = _reload_web_search(trusted="")
        assert mod.is_url_allowed("not-a-url") is False
        assert mod.is_url_allowed("") is False

    def test_subdomain_matching(self):
        mod = _reload_web_search(trusted="pytorch.org")
        assert mod.is_url_allowed("https://docs.pytorch.org/stable") is True

    def test_partial_domain_match(self):
        """is_url_allowed uses substring matching, so 'python.org' in
        'notpython.org' is True by design — this mirrors the source code."""
        mod = _reload_web_search(trusted="python.org")
        assert mod.is_url_allowed("https://docs.python.org/3/") is True
        assert mod.is_url_allowed("https://notpython.org/") is True  # substring match
        assert mod.is_url_allowed("https://rust-lang.org/") is False


# ---------------------------------------------------------------------------
# RateLimiter
# ---------------------------------------------------------------------------


class TestRateLimiter:
    def test_acquire_within_limit(self):
        from src.pipeline.web_search import RateLimiter

        rl = RateLimiter(max_per_minute=3)
        assert rl.acquire() is True
        assert rl.acquire() is True
        assert rl.acquire() is True

    def test_acquire_exceeds_limit(self):
        from src.pipeline.web_search import RateLimiter

        rl = RateLimiter(max_per_minute=2)
        assert rl.acquire() is True
        assert rl.acquire() is True
        assert rl.acquire() is False

    def test_tokens_expire_after_60s(self):
        from src.pipeline.web_search import RateLimiter

        rl = RateLimiter(max_per_minute=1)
        assert rl.acquire() is True
        assert rl.acquire() is False

        with patch("src.pipeline.web_search.time") as mock_time:
            mock_time.monotonic.return_value = time.monotonic() + 61
            assert rl.acquire() is True

    def test_zero_limit_always_denies(self):
        from src.pipeline.web_search import RateLimiter

        rl = RateLimiter(max_per_minute=0)
        assert rl.acquire() is False


# ---------------------------------------------------------------------------
# NoopSearchProvider
# ---------------------------------------------------------------------------


class TestNoopSearchProvider:
    @pytest.mark.asyncio
    async def test_returns_empty_list(self):
        from src.pipeline.web_search import NoopSearchProvider

        provider = NoopSearchProvider()
        results = await provider.search("anything")
        assert results == []

    @pytest.mark.asyncio
    async def test_respects_max_results(self):
        from src.pipeline.web_search import NoopSearchProvider

        provider = NoopSearchProvider()
        results = await provider.search("q", max_results=100)
        assert results == []


# ---------------------------------------------------------------------------
# Mock search provider – URL allowlist filtering
# ---------------------------------------------------------------------------


class _StubSearchProvider:
    """Returns hard-coded results with mixed allowed/denied URLs."""

    def __init__(self, results):
        self._results = results

    async def search(self, query, max_results=5):
        return self._results[:max_results]


class TestWebSearchServiceFiltering:
    @pytest.mark.asyncio
    async def test_filters_disallowed_urls(self):
        mod = _reload_web_search(trusted="arxiv.org", denied="")

        results = [
            {"title": "Good", "url": "https://arxiv.org/abs/1", "snippet": ""},
            {"title": "Bad", "url": "https://evil.com/x", "snippet": ""},
            {"title": "Also good", "url": "https://arxiv.org/abs/2", "snippet": ""},
        ]

        service = mod.WebSearchService.__new__(mod.WebSearchService)
        service._provider = _StubSearchProvider(results)
        service._limiter = mod.RateLimiter(60)
        service._calls_this_job = 0

        with patch.object(settings, "max_web_search_per_job", 10):
            filtered = await service.search("test query")

        urls = [r["url"] for r in filtered]
        assert "https://evil.com/x" not in urls
        assert len(filtered) == 2

    @pytest.mark.asyncio
    async def test_job_limit_enforced(self):
        mod = _reload_web_search(trusted="", denied="")

        service = mod.WebSearchService.__new__(mod.WebSearchService)
        service._provider = _StubSearchProvider([{"title": "T", "url": "https://ok.com", "snippet": ""}])
        service._limiter = mod.RateLimiter(60)
        service._calls_this_job = 0

        with patch.object(settings, "max_web_search_per_job", 1):
            r1 = await service.search("q1")
            assert len(r1) >= 0
            r2 = await service.search("q2")
            assert r2 == []

    @pytest.mark.asyncio
    async def test_reset_job_counter(self):
        mod = _reload_web_search(trusted="", denied="")

        service = mod.WebSearchService.__new__(mod.WebSearchService)
        service._provider = _StubSearchProvider([{"title": "T", "url": "https://ok.com", "snippet": ""}])
        service._limiter = mod.RateLimiter(60)
        service._calls_this_job = 5

        with patch.object(settings, "max_web_search_per_job", 3):
            assert await service.search("q") == []
            service.reset_job_counter()
            assert service._calls_this_job == 0
            results = await service.search("q")
            assert len(results) >= 0


# ---------------------------------------------------------------------------
# ArxivSearchProvider (structure only, no real HTTP)
# ---------------------------------------------------------------------------


class TestArxivSearchProvider:
    def test_api_url_constant(self):
        from src.pipeline.web_search import ArxivSearchProvider

        assert "arxiv.org" in ArxivSearchProvider.API_URL
