"""Shared test fixtures."""

from __future__ import annotations

import pytest


@pytest.fixture
def mock_cookies() -> dict[str, str]:
    """A realistic-looking cookie dict for testing."""
    return {
        "_m_h5_tk": "abc123def456_1710000000000",
        "_m_h5_tk_enc": "enc_token_value",
        "unb": "3888777108",
        "cookie2": "test_cookie2",
        "sgcookie": "test_sgcookie",
        "t": "test_t_value",
        "isg": "test_isg",
    }


@pytest.fixture
def mock_api_response() -> dict:
    """A successful mtop API response (matches real API structure)."""
    return {
        "ret": ["SUCCESS::调用成功"],
        "data": {
            "resultList": [
                {
                    "data": {
                        "item": {
                            "main": {
                                "exContent": {
                                    "area": "杭州",
                                    "detailParams": {
                                        "itemId": "123456",
                                        "title": "iPhone 15 Pro 256G",
                                        "soldPrice": "599900",
                                        "userNick": "测试卖家",
                                    },
                                },
                                "clickParam": {
                                    "args": {
                                        "id": "123456",
                                        "price": "599900",
                                        "p_city": "杭州市",
                                        "seller_id": "user001",
                                    }
                                },
                            }
                        }
                    }
                }
            ]
        },
    }
