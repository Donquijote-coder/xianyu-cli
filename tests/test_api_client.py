"""Tests for the API client response parsing."""

from xianyu_cli.core.api_client import (
    ApiError,
    GoofishApiClient,
    RateLimitError,
    TokenExpiredError,
)

import pytest


def test_parse_response_success():
    client = GoofishApiClient.__new__(GoofishApiClient)
    body = {
        "ret": ["SUCCESS::调用成功"],
        "data": {"itemId": "123"},
    }
    result = client._parse_response(body)
    assert result == {"itemId": "123"}


def test_parse_response_token_expired():
    client = GoofishApiClient.__new__(GoofishApiClient)
    body = {
        "ret": ["FAIL_SYS_TOKEN_EXOIRED::令牌过期"],
        "data": {},
    }
    with pytest.raises(TokenExpiredError):
        client._parse_response(body)


def test_parse_response_rate_limit():
    client = GoofishApiClient.__new__(GoofishApiClient)
    body = {
        "ret": ["FAIL_SYS_ILLEGAL_ACCESS::非法请求"],
        "data": {},
    }
    with pytest.raises(RateLimitError):
        client._parse_response(body)


def test_parse_response_generic_error():
    client = GoofishApiClient.__new__(GoofishApiClient)
    body = {
        "ret": ["FAIL_BIZ::业务错误"],
        "data": {},
    }
    with pytest.raises(ApiError):
        client._parse_response(body)


def test_token_extraction(mock_cookies):
    client = GoofishApiClient(cookies=mock_cookies)
    assert client.token == "abc123def456"
