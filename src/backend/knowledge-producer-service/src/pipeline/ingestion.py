"""Multi-format ingestion: .md, .pdf, .json, URL (§9 p.5.8).

Produces a RawDocument for the pipeline regardless of source format.
"""

from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path
from typing import Any, Dict, Optional

import httpx

from ..config import settings
from ..logger.setup import get_logger
from ..schemas import RawDocument

logger = get_logger(__name__)


def _file_hash(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


# ── Parsers ──────────────────────────────────────────────────────


def parse_markdown(path: Path) -> RawDocument:
    """Parse a .md file into RawDocument."""
    data = path.read_bytes()
    if len(data) > settings.max_ingest_file_bytes:
        raise ValueError(f"File too large: {len(data)} bytes (max {settings.max_ingest_file_bytes})")
    text = data.decode("utf-8", errors="replace")
    return RawDocument(
        source_uri=str(path),
        mime="text/markdown",
        text=text,
        metadata={"file_hash": _file_hash(data), "file_name": path.name},
    )


def parse_json_file(path: Path) -> RawDocument:
    """Parse a .json file: extract text from structured content."""
    data = path.read_bytes()
    if len(data) > settings.max_ingest_file_bytes:
        raise ValueError(f"File too large: {len(data)} bytes")
    obj = json.loads(data)
    if isinstance(obj, dict):
        text = json.dumps(obj, ensure_ascii=False, indent=2)
    elif isinstance(obj, list):
        text = json.dumps(obj, ensure_ascii=False, indent=2)
    else:
        text = str(obj)
    return RawDocument(
        source_uri=str(path),
        mime="application/json",
        text=text,
        metadata={"file_hash": _file_hash(data), "file_name": path.name},
    )


def parse_pdf(path: Path) -> RawDocument:
    """Parse a .pdf file: basic text extraction.

    Uses pdfplumber if available, otherwise falls back to reading raw bytes
    with a warning. Full OCR is deferred to v2.
    """
    data = path.read_bytes()
    if len(data) > settings.max_ingest_file_bytes:
        raise ValueError(f"File too large: {len(data)} bytes")

    text = ""
    try:
        import pdfplumber  # type: ignore[import-untyped]

        with pdfplumber.open(path) as pdf:
            pages = []
            for page in pdf.pages:
                page_text = page.extract_text()
                if page_text:
                    pages.append(page_text)
            text = "\n\n".join(pages)
    except ImportError:
        logger.warning("pdfplumber not installed, PDF text extraction unavailable")
        text = ""

    if not text.strip():
        raise ValueError("Empty text extracted from PDF (OCR may be needed)")

    return RawDocument(
        source_uri=str(path),
        mime="application/pdf",
        text=text,
        metadata={"file_hash": _file_hash(data), "file_name": path.name},
    )


async def fetch_url(url: str) -> RawDocument:
    """Fetch URL content, strip HTML to plain text."""
    async with httpx.AsyncClient(timeout=settings.fetch_url_timeout) as client:
        resp = await client.get(url, follow_redirects=True)
        resp.raise_for_status()

        content_length = len(resp.content)
        if content_length > settings.max_fetch_url_bytes:
            raise ValueError(f"URL content too large: {content_length} bytes")

        content_type = resp.headers.get("content-type", "")
        text = resp.text

        if "html" in content_type:
            text = _strip_html(text)

        return RawDocument(
            source_uri=url,
            mime=content_type.split(";")[0].strip(),
            text=text,
            metadata={"url": url, "status_code": resp.status_code},
        )


def _strip_html(html: str) -> str:
    """Minimal HTML → plain text conversion."""
    text = re.sub(r"<script[^>]*>.*?</script>", "", html, flags=re.DOTALL | re.IGNORECASE)
    text = re.sub(r"<style[^>]*>.*?</style>", "", text, flags=re.DOTALL | re.IGNORECASE)
    text = re.sub(r"<[^>]+>", " ", text)
    text = re.sub(r"\s+", " ", text)
    return text.strip()


# ── Dispatcher ───────────────────────────────────────────────────


PARSERS = {
    ".md": parse_markdown,
    ".json": parse_json_file,
    ".pdf": parse_pdf,
}


def ingest_file(path: str) -> RawDocument:
    """Dispatch to the appropriate parser based on file extension."""
    p = Path(path)
    ext = p.suffix.lower()
    parser = PARSERS.get(ext)
    if parser is None:
        raise ValueError(f"Unsupported file format: {ext}")
    return parser(p)


async def ingest_url(url: str) -> RawDocument:
    """Fetch and parse URL content."""
    return await fetch_url(url)


def parse_uploaded_file(filename: str, content: bytes) -> RawDocument:
    """Parse an in-memory uploaded file (from HTTP multipart upload).

    Supports .md, .txt, .json, .pdf based on the filename extension.
    """
    if len(content) > settings.max_ingest_file_bytes:
        raise ValueError(
            f"File too large: {len(content)} bytes (max {settings.max_ingest_file_bytes})"
        )

    ext = Path(filename).suffix.lower()
    file_hash = _file_hash(content)
    meta = {"file_hash": file_hash, "file_name": filename}

    if ext in (".md", ".txt"):
        text = content.decode("utf-8", errors="replace")
        return RawDocument(
            source_uri=f"upload://{filename}",
            mime="text/markdown" if ext == ".md" else "text/plain",
            text=text,
            metadata=meta,
        )

    if ext == ".json":
        obj = json.loads(content)
        if isinstance(obj, dict):
            text = obj.get("text") or obj.get("content") or json.dumps(obj, ensure_ascii=False)
        elif isinstance(obj, list):
            text = "\n".join(
                item.get("text", "") if isinstance(item, dict) else str(item) for item in obj
            )
        else:
            text = str(obj)
        return RawDocument(source_uri=f"upload://{filename}", mime="application/json", text=text, metadata=meta)

    if ext == ".pdf":
        try:
            import io
            import pdfplumber
            with pdfplumber.open(io.BytesIO(content)) as pdf:
                text = "\n".join(page.extract_text() or "" for page in pdf.pages)
        except ImportError:
            logger.warning("pdfplumber not installed, PDF text extraction unavailable")
            text = content.decode("utf-8", errors="replace")
        return RawDocument(
            source_uri=f"upload://{filename}", mime="application/pdf", text=text, metadata=meta
        )

    raise ValueError(f"Unsupported file format: {ext}. Supported: .md, .txt, .json, .pdf")
