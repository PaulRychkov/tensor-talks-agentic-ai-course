"""PII detection and filtering module for 152-ФЗ compliance (§10.11).

Two-level filter:
  Level 1 (deterministic): Regex patterns for email, phone, card, ФИО, etc.
  Level 2 (LLM-based): Classification of subtle/indirect PII.

Architecture note (§10.11 Variant A):
  PII check should run in BFF BEFORE publishing to Kafka, so no PII ever
  reaches chat-crud-service or the LLM provider. This module provides the
  detection logic; the calling code decides what to do.
"""

import json
import re
from enum import Enum
from typing import Optional, Tuple, List

from ..logger import get_logger

logger = get_logger(__name__)


class PIICategory(str, Enum):
    EMAIL = "email"
    PHONE = "phone"
    CARD = "card"
    FULL_NAME = "full_name"
    DATE_OF_BIRTH = "date_of_birth"
    COMPANY = "company"
    PASSPORT = "passport"
    INN = "inn"
    SNILS = "snils"


# ── Level 1: Regex patterns ───────────────────────────────────────────────────

_PII_PATTERNS: List[Tuple[re.Pattern, PIICategory]] = [
    # 1. Email — unambiguous (contains @)
    (re.compile(r"[\w\.+-]+@[\w\.-]+\.\w+", re.IGNORECASE), PIICategory.EMAIL),
    # 2. Credit/debit card — 16 digits in 4×4 groups or continuous (check BEFORE generic phone)
    (re.compile(r"\b(?:\d{4}[ \-]?){3}\d{4}\b"), PIICategory.CARD),
    # 3. INN — requires keyword to avoid false-positives with digit sequences
    (re.compile(r"\bИНН\s*[:—\-]?\s*\d{10,12}\b", re.IGNORECASE), PIICategory.INN),
    # 4. СНИЛС — XXX-XXX-XXX-XX pattern (use dash separator, not optional space)
    (re.compile(r"\b\d{3}[\s\-]\d{3}[\s\-]\d{3}[\s\-]\d{2}\b"), PIICategory.SNILS),
    # 5. Russian passport — 4 digits space 6 digits OR 2+2+6 with spaces
    (re.compile(r"\b\d{4}\s\d{6}\b|\b\d{2}\s\d{2}\s\d{6}\b"), PIICategory.PASSPORT),
    # 6. Russian phone — must start with +7 or 8 (to avoid catching INN/SNILS digit runs)
    (
        re.compile(
            r"(?:\+7|8)[\s\-]?\(?\d{3}\)?[\s\-]?\d{3}[\s\-]?\d{2}[\s\-]?\d{2}"
        ),
        PIICategory.PHONE,
    ),
    # 7. Full name — REMOVED. A bare "Word Word" pattern matches ML terms, researcher
    #    names in citations, and Cyrillic website names (e.g. "Хабр Хабр"), causing
    #    constant false positives in interview answers. Pattern 7b below covers actual
    #    self-identification; Level 2 LLM filter handles edge cases.
    # 7b. Self-identification by name: "меня зовут Паша", "я Паша", "меня Паша зовут",
    #     "моё имя Паша", "зовут меня Паша" — case-insensitive, name may be lowercase.
    #
    #     IMPORTANT: The standalone-я alternative uses (?<!\w) to ensure "я" is a
    #     separate word, NOT the end-of-word letter in Russian words like "выявления"
    #     (which would falsely match "я выбросов." via "выявления выбросов.").
    (
        re.compile(
            r"(?:"
            r"меня\s+зовут\s+[А-ЯЁа-яёA-Za-z][а-яёa-z]{1,20}"
            r"|зовут\s+меня\s+[А-ЯЁа-яёA-Za-z][а-яёa-z]{1,20}"
            r"|моё?\s+имя\s+[А-ЯЁа-яёA-Za-z][а-яёa-z]{1,20}"
            r"|я\s+называюсь\s+[А-ЯЁа-яёA-Za-z][а-яёa-z]{1,20}"
            r"|меня\s+[А-ЯЁа-яёA-Za-z][а-яёa-z]{1,20}\s+зовут"
            r"|(?<!\w)я(?!\w)\s+[А-ЯЁ][а-яё]{1,20}(?=\s*[,.\n]|\s+кстати|\s+буду|\s+хочу|\s+работаю|\s+учусь|$)"
            r")",
            re.IGNORECASE,
        ),
        PIICategory.FULL_NAME,
    ),
    # 8. Date of birth in context
    (
        re.compile(
            r"(?:родил(?:ся|ась)|дата рождения|д\.?\s?р\.?)\s*[:—\-]?\s*"
            r"\d{1,2}[\.\/\-]\d{1,2}[\.\/\-]\d{2,4}",
            re.IGNORECASE,
        ),
        PIICategory.DATE_OF_BIRTH,
    ),
]

# Company blacklist (configurable via company_blacklist.json or env; §10.11)
_DEFAULT_COMPANY_KEYWORDS: List[str] = [
    "сбер", "сбэр", "сбербанк", "яндекс", "yandex", "авито", "avito",
    "вконтакте", "vk", "mail.ru", "мейл.ру", "тинькофф", "tinkoff",
    "т-банк", "ozon", "озон", "wildberries", "вайлдберриз",
    "мтс", "мегафон", "билайн", "beeline", "ростелеком", "rostelecom",
    "касперский", "kaspersky", "positive technologies",
    "газпром", "gazprom", "роснефть", "лукойл", "lukoil",
    "альфа-банк", "alfa-bank", "втб", "vtb", "райффайзен",
    "huawei", "хуавей", "samsung", "самсунг", "google", "гугл",
    "amazon", "амазон", "meta", "facebook", "фейсбук",
    "microsoft", "майкрософт", "apple", "эпл",
    "nvidia", "нвидиа", "intel", "интел", "amd",
    "x5", "магнит", "пятёрочка", "перекрёсток",
    "сколково", "skolkovo", "иннополис", "innopolis",
]

# Technical terms that are NOT company PII (whitelist)
_TECH_WHITELIST: List[str] = [
    "google colab", "google cloud", "google scholar",
    "amazon web services", "amazon s3", "aws",
    "microsoft azure", "microsoft visual studio",
    "apple silicon", "apple m1", "apple m2",
    "intel core", "intel xeon", "amd ryzen",
    "nvidia cuda", "nvidia gpu",
    "сберегательн",  # covers "сберегательный счёт" etc.
    "газовая",       # covers "газовая промышленность"
    "мтс-маклер",    # financial, not telecom
]

_company_keywords: List[str] = list(_DEFAULT_COMPANY_KEYWORDS)


def load_company_blacklist(path: str) -> None:
    """Load custom company blacklist from a JSON file (§10.11)."""
    global _company_keywords
    try:
        with open(path, encoding="utf-8") as f:
            data = json.load(f)
        if isinstance(data, list):
            _company_keywords = [kw.lower() for kw in data]
            logger.info("Company blacklist loaded", count=len(_company_keywords), path=path)
    except Exception as exc:
        logger.warning("Failed to load company blacklist", path=path, error=str(exc))


class PIIFilterResult:
    """Result of a PII check."""

    def __init__(self, detected: bool, category: Optional[PIICategory] = None, reason: str = "", masked_text: str = ""):
        self.detected = detected
        self.category = category
        self.reason = reason
        self.masked_text = masked_text  # text with PII fragments replaced by placeholders

    def __bool__(self) -> bool:
        return self.detected


def _is_whitelisted(text_lower: str, company: str) -> bool:
    """Return True if the company keyword appears only in a whitelisted technical context."""
    for phrase in _TECH_WHITELIST:
        if phrase in text_lower and company in phrase:
            return True
    return False


def mask_pii_regex(text: str) -> tuple[str, list[tuple[str, str]]]:
    """Replace PII fragments in text with category placeholders.

    Returns (masked_text, replacements) where replacements is a list of
    (original_fragment, placeholder) tuples.
    """
    masked = text
    replacements: list[tuple[str, str]] = []

    for pattern, category in _PII_PATTERNS:
        placeholder = f"[{category.value}]"
        for match in pattern.finditer(masked):
            original = match.group(0)
            if (original, placeholder) not in replacements:
                replacements.append((original, placeholder))
        masked = pattern.sub(placeholder, masked)

    # Company check
    text_lower_check = text.lower()
    for company in _company_keywords:
        pat = r"(?<!\w)" + re.escape(company)
        if re.search(pat, text_lower_check):
            if not _is_whitelisted(text_lower_check, company):
                placeholder = "[компания]"
                masked = re.sub(pat, placeholder, masked, flags=re.IGNORECASE)
                replacements.append((company, placeholder))

    return masked, replacements


def check_pii_regex(text: str) -> PIIFilterResult:
    """Check text for PII using Level 1 regex filters (§10.11).

    Returns a PIIFilterResult with detected=True if any PII pattern matches.
    Also returns masked_text with the detected fragments replaced by placeholders.
    """
    if not text or not text.strip():
        return PIIFilterResult(detected=False)

    for pattern, category in _PII_PATTERNS:
        if pattern.search(text):
            logger.debug("PII regex match", category=category.value)
            masked, _ = mask_pii_regex(text)
            return PIIFilterResult(
                detected=True,
                category=category,
                reason=f"Обнаружено: {category.value}",
                masked_text=masked,
            )

    # Company check (case-insensitive, prefix-aware to catch inflections like Яндексе/Сбере)
    text_lower = text.lower()
    for company in _company_keywords:
        pattern = r"(?<!\w)" + re.escape(company)
        if re.search(pattern, text_lower):
            if not _is_whitelisted(text_lower, company):
                logger.debug("PII company match", company=company)
                masked, _ = mask_pii_regex(text)
                return PIIFilterResult(
                    detected=True,
                    category=PIICategory.COMPANY,
                    reason="Обнаружено название компании-работодателя",
                    masked_text=masked,
                )

    return PIIFilterResult(detected=False)


# ── Level 2: LLM-based classification ────────────────────────────────────────

PII_CHECK_PROMPT_TEMPLATE = """\
Ты — классификатор персональных данных.
Определи, содержит ли текст пользователя персональные данные или конфиденциальную информацию:
- ФИО или части имени
- Упоминания конкретных компаний-работодателей (текущих или бывших)
- Даты, связанные с биографией (дата рождения, дата увольнения и т.п.)
- Адреса, города в контексте места проживания/работы
- Номера документов, банковских продуктов
- Любую информацию, позволяющую идентифицировать конкретного человека

Если персональные данные обнаружены, перечисли конкретные фрагменты, которые их содержат, \
и укажи тип каждого ([имя], [компания], [дата], [адрес], [документ], [телефон], [email]).
Создай маскированную версию текста, заменив каждый PII-фрагмент соответствующим плейсхолдером.

Отвечай СТРОГО в формате JSON:
{{"contains_pii": true/false, "reason": "краткое объяснение найденных ПДн", "masked_text": "текст с заменёнными фрагментами или пустая строка если ПДн нет"}}

Текст пользователя:
{user_text}"""


async def check_pii_llm(text: str, llm_client) -> PIIFilterResult:
    """Check text for subtle PII using LLM classification (Level 2, §10.11).

    Only called when Level 1 regex passes (no obvious PII found).
    Uses a short, cheap classification prompt with temperature=0.
    Returns masked_text with PII fragments replaced by type placeholders.
    """
    from ..models.llm_outputs import PIICheckResult

    if not text or not text.strip():
        return PIIFilterResult(detected=False)

    prompt = PII_CHECK_PROMPT_TEMPLATE.format(user_text=text[:2000])

    try:
        fallback = PIICheckResult(contains_pii=False, reason="llm_error")
        result = await llm_client.generate_structured(
            prompt=prompt,
            response_model=PIICheckResult,
            fallback=fallback,
            low_temp=0.0,
        )
        if result.contains_pii:
            masked = getattr(result, "masked_text", None) or text
            return PIIFilterResult(
                detected=True,
                category=None,
                reason=f"LLM: {result.reason}",
                masked_text=masked,
            )
        return PIIFilterResult(detected=False)
    except Exception as exc:
        logger.warning("LLM PII check failed, treating as safe", error=str(exc))
        return PIIFilterResult(detected=False)


# ── Public API ────────────────────────────────────────────────────────────────

# System message sent to the user when PII is detected
PII_REJECTION_MESSAGE = (
    "Ваше сообщение содержит информацию, похожую на персональные данные. "
    "Пожалуйста, переформулируйте ответ без упоминания конкретных имён, "
    "компаний, дат и других идентифицирующих сведений. "
    "Это необходимо для вашей безопасности."
)

# Placeholder stored in DB instead of the original PII-containing text
PII_REDACTED_PLACEHOLDER = "[Сообщение содержало конфиденциальную информацию и было отклонено]"
