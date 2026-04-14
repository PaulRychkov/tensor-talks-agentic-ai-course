"""Tests for src/pipeline/ingestion.py – file parsers and helpers."""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import patch

import pytest

from src.config import settings


# ---------------------------------------------------------------------------
# parse_markdown
# ---------------------------------------------------------------------------


class TestParseMarkdown:
    def test_golden_file(self, golden_md: Path):
        from src.pipeline.ingestion import parse_markdown

        doc = parse_markdown(golden_md)
        assert doc.mime == "text/markdown"
        assert "Neural Networks" in doc.text
        assert doc.metadata["file_name"] == "notes.md"
        assert "file_hash" in doc.metadata

    def test_source_uri_matches_path(self, golden_md: Path):
        from src.pipeline.ingestion import parse_markdown

        doc = parse_markdown(golden_md)
        assert doc.source_uri == str(golden_md)

    def test_file_too_large_raises(self, tmp_path: Path):
        big = tmp_path / "huge.md"
        big.write_bytes(b"x" * (settings.max_ingest_file_bytes + 1))

        from src.pipeline.ingestion import parse_markdown

        with pytest.raises(ValueError, match="too large"):
            parse_markdown(big)

    def test_non_utf8_replaced(self, tmp_path: Path):
        md = tmp_path / "bad.md"
        md.write_bytes(b"hello \xff world")

        from src.pipeline.ingestion import parse_markdown

        doc = parse_markdown(md)
        assert "hello" in doc.text


# ---------------------------------------------------------------------------
# parse_json_file
# ---------------------------------------------------------------------------


class TestParseJsonFile:
    def test_dict_json(self, tmp_path: Path):
        f = tmp_path / "data.json"
        f.write_text(json.dumps({"key": "value"}), encoding="utf-8")

        from src.pipeline.ingestion import parse_json_file

        doc = parse_json_file(f)
        assert doc.mime == "application/json"
        assert '"key"' in doc.text

    def test_list_json(self, tmp_path: Path):
        f = tmp_path / "arr.json"
        f.write_text(json.dumps([1, 2, 3]), encoding="utf-8")

        from src.pipeline.ingestion import parse_json_file

        doc = parse_json_file(f)
        assert "1" in doc.text

    def test_scalar_json(self, tmp_path: Path):
        f = tmp_path / "scalar.json"
        f.write_text('"just a string"', encoding="utf-8")

        from src.pipeline.ingestion import parse_json_file

        doc = parse_json_file(f)
        assert "just a string" in doc.text

    def test_too_large_json(self, tmp_path: Path):
        f = tmp_path / "big.json"
        f.write_bytes(b"[" + b"1," * (settings.max_ingest_file_bytes) + b"1]")

        from src.pipeline.ingestion import parse_json_file

        with pytest.raises(ValueError, match="too large"):
            parse_json_file(f)


# ---------------------------------------------------------------------------
# parse_pdf – empty extraction
# ---------------------------------------------------------------------------


class TestParsePdf:
    def test_empty_extraction_raises(self, tmp_path: Path):
        pdf = tmp_path / "empty.pdf"
        pdf.write_bytes(b"%PDF-1.4 minimal")

        from src.pipeline.ingestion import parse_pdf

        with pytest.raises(ValueError, match="Empty text"):
            parse_pdf(pdf)

    def test_too_large_pdf(self, tmp_path: Path):
        pdf = tmp_path / "big.pdf"
        pdf.write_bytes(b"x" * (settings.max_ingest_file_bytes + 1))

        from src.pipeline.ingestion import parse_pdf

        with pytest.raises(ValueError, match="too large"):
            parse_pdf(pdf)


# ---------------------------------------------------------------------------
# _strip_html
# ---------------------------------------------------------------------------


class TestStripHtml:
    def test_removes_tags(self):
        from src.pipeline.ingestion import _strip_html

        assert _strip_html("<p>hello</p>") == "hello"

    def test_removes_script_blocks(self):
        from src.pipeline.ingestion import _strip_html

        html = '<div>keep</div><script type="text/javascript">alert(1)</script><div>this</div>'
        result = _strip_html(html)
        assert "alert" not in result
        assert "keep" in result and "this" in result

    def test_removes_style_blocks(self):
        from src.pipeline.ingestion import _strip_html

        html = "<style>.x{color:red}</style><p>visible</p>"
        result = _strip_html(html)
        assert "color" not in result
        assert "visible" in result

    def test_collapses_whitespace(self):
        from src.pipeline.ingestion import _strip_html

        html = "<p>  hello   world  </p>"
        assert _strip_html(html) == "hello world"

    def test_empty_input(self):
        from src.pipeline.ingestion import _strip_html

        assert _strip_html("") == ""


# ---------------------------------------------------------------------------
# ingest_file dispatcher
# ---------------------------------------------------------------------------


class TestIngestFile:
    def test_dispatches_markdown(self, golden_md: Path):
        from src.pipeline.ingestion import ingest_file

        doc = ingest_file(str(golden_md))
        assert doc.mime == "text/markdown"

    def test_dispatches_json(self, tmp_path: Path):
        f = tmp_path / "test.json"
        f.write_text('{"a":1}', encoding="utf-8")

        from src.pipeline.ingestion import ingest_file

        doc = ingest_file(str(f))
        assert doc.mime == "application/json"

    def test_unsupported_extension_raises(self, tmp_path: Path):
        f = tmp_path / "data.csv"
        f.write_text("a,b,c", encoding="utf-8")

        from src.pipeline.ingestion import ingest_file

        with pytest.raises(ValueError, match="Unsupported file format"):
            ingest_file(str(f))

    def test_extension_case_insensitive(self, tmp_path: Path):
        f = tmp_path / "readme.MD"
        f.write_text("# Hi", encoding="utf-8")

        from src.pipeline.ingestion import ingest_file

        doc = ingest_file(str(f))
        assert doc.mime == "text/markdown"


# ---------------------------------------------------------------------------
# File hash determinism
# ---------------------------------------------------------------------------


class TestFileHash:
    def test_same_content_same_hash(self, tmp_path: Path):
        from src.pipeline.ingestion import parse_markdown

        a = tmp_path / "a.md"
        b = tmp_path / "b.md"
        a.write_text("same", encoding="utf-8")
        b.write_text("same", encoding="utf-8")

        da = parse_markdown(a)
        db = parse_markdown(b)
        assert da.metadata["file_hash"] == db.metadata["file_hash"]

    def test_different_content_different_hash(self, tmp_path: Path):
        from src.pipeline.ingestion import parse_markdown

        a = tmp_path / "a.md"
        b = tmp_path / "b.md"
        a.write_text("alpha", encoding="utf-8")
        b.write_text("bravo", encoding="utf-8")

        da = parse_markdown(a)
        db = parse_markdown(b)
        assert da.metadata["file_hash"] != db.metadata["file_hash"]
