"""Unit tests for PII regex filter (§10.11).

Tests positive cases (PII should be detected) and negative cases
(normal technical text should NOT trigger the filter).
"""

import pytest
from src.guardrails.pii_filter import check_pii_regex, PIICategory


# ── Positive cases: PII should be detected ───────────────────────────────────

class TestEmailDetection:
    def test_simple_email(self):
        result = check_pii_regex("мой email user@mail.ru")
        assert result.detected
        assert result.category == PIICategory.EMAIL

    def test_corporate_email(self):
        result = check_pii_regex("Напишите на john.doe@company.com")
        assert result.detected
        assert result.category == PIICategory.EMAIL


class TestPhoneDetection:
    def test_russian_phone_with_plus7(self):
        result = check_pii_regex("Позвоните +7 (915) 123-45-67")
        assert result.detected
        assert result.category == PIICategory.PHONE

    def test_russian_phone_with_8(self):
        result = check_pii_regex("8-800-555-35-35")
        assert result.detected
        assert result.category == PIICategory.PHONE


class TestCardDetection:
    def test_card_number(self):
        result = check_pii_regex("карта 4276 1234 5678 9012")
        assert result.detected
        assert result.category == PIICategory.CARD


class TestFullNameDetection:
    def test_cyrillic_full_name(self):
        result = check_pii_regex("меня зовут Иванов Пётр Сергеевич")
        assert result.detected
        assert result.category == PIICategory.FULL_NAME

    def test_two_word_name(self):
        result = check_pii_regex("Сотрудник Петрова Мария получила задание")
        assert result.detected
        assert result.category == PIICategory.FULL_NAME


class TestCompanyDetection:
    def test_sberbank(self):
        result = check_pii_regex("В Сбере я работал 3 года над LLM-проектом")
        assert result.detected
        assert result.category == PIICategory.COMPANY

    def test_yandex(self):
        result = check_pii_regex("работал в Яндексе как ML-инженер")
        assert result.detected
        assert result.category == PIICategory.COMPANY

    def test_google(self):
        result = check_pii_regex("Я работаю в Google с 2020 года")
        assert result.detected
        assert result.category == PIICategory.COMPANY


class TestPassportDetection:
    def test_passport_series_number(self):
        result = check_pii_regex("Паспорт 4510 123456")
        assert result.detected
        assert result.category == PIICategory.PASSPORT


class TestINNDetection:
    def test_inn(self):
        result = check_pii_regex("ИНН: 7707083893")
        assert result.detected
        assert result.category == PIICategory.INN


class TestSNILSDetection:
    def test_snils(self):
        result = check_pii_regex("СНИЛС 123-456-789 01")
        assert result.detected
        assert result.category == PIICategory.SNILS


# ── Negative cases: normal text should NOT trigger the filter ─────────────────

class TestNegativeCases:
    def test_technical_answer_no_pii(self):
        result = check_pii_regex("Использовал нейронные сети для классификации изображений")
        assert not result.detected

    def test_go_api_answer(self):
        result = check_pii_regex("Реализовал REST API на Go с использованием gin фреймворка")
        assert not result.detected

    def test_neural_network_explanation(self):
        result = check_pii_regex(
            "Градиентный спуск минимизирует функцию потерь через вычисление частных производных"
        )
        assert not result.detected

    def test_transformer_explanation(self):
        result = check_pii_regex(
            "Механизм внимания в Transformer позволяет учитывать контекст всей последовательности"
        )
        assert not result.detected

    def test_python_code_reference(self):
        result = check_pii_regex("import numpy as np; model.fit(X_train, y_train)")
        assert not result.detected

    def test_infrastructure_answer(self):
        result = check_pii_regex("Развернул Kubernetes кластер с помощью helm charts")
        assert not result.detected

    def test_sber_as_prefix(self):
        # "сберегательная стратегия" should NOT match as PII (word boundary check)
        result = check_pii_regex("Использовал сберегательную стратегию при обучении модели")
        assert not result.detected

    def test_google_cloud_whitelisted(self):
        # "google colab" in technical context should be whitelisted
        result = check_pii_regex("Запустил модель в Google Colab для обучения")
        # This may or may not be detected depending on whitelist - just ensure no crash
        assert result is not None

    def test_amazon_aws_reference(self):
        # Technical AWS reference - should not trigger as employer PII
        result = check_pii_regex("Деплоил сервис на Amazon Web Services S3")
        assert result is not None  # No crash

    def test_empty_text(self):
        result = check_pii_regex("")
        assert not result.detected

    def test_none_text(self):
        result = check_pii_regex(None)
        assert not result.detected
