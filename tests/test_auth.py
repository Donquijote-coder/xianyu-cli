"""Tests for credential management."""

import json
from pathlib import Path

from xianyu_cli.utils.credential import (
    Credential,
    delete_credential,
    load_credential,
    save_credential,
)


def test_credential_not_expired(mock_cookies):
    cred = Credential(cookies=mock_cookies, source="test")
    assert not cred.is_expired(ttl_hours=24)


def test_credential_expired(mock_cookies):
    cred = Credential(
        cookies=mock_cookies,
        source="test",
        saved_at="2020-01-01T00:00:00+00:00",
    )
    assert cred.is_expired(ttl_hours=24)


def test_credential_m_h5_tk(mock_cookies):
    cred = Credential(cookies=mock_cookies)
    assert cred.m_h5_tk == "abc123def456"


def test_credential_m_h5_tk_missing():
    cred = Credential(cookies={})
    assert cred.m_h5_tk is None


def test_save_and_load_credential(tmp_path, mock_cookies):
    path = tmp_path / "cred.json"
    cred = Credential(cookies=mock_cookies, user_id="123", source="test")
    save_credential(cred, path=path)

    loaded = load_credential(path=path)
    assert loaded is not None
    assert loaded.user_id == "123"
    assert loaded.cookies["unb"] == "3888777108"


def test_load_credential_missing(tmp_path):
    path = tmp_path / "nonexistent.json"
    assert load_credential(path=path) is None


def test_delete_credential(tmp_path, mock_cookies):
    path = tmp_path / "cred.json"
    save_credential(Credential(cookies=mock_cookies), path=path)
    assert path.exists()

    assert delete_credential(path=path) is True
    assert not path.exists()


def test_delete_credential_missing(tmp_path):
    path = tmp_path / "nonexistent.json"
    assert delete_credential(path=path) is False


def test_credential_roundtrip(mock_cookies):
    cred = Credential(
        cookies=mock_cookies,
        user_id="42",
        nickname="测试用户",
        source="browser",
    )
    d = cred.to_dict()
    restored = Credential.from_dict(d)
    assert restored.user_id == "42"
    assert restored.nickname == "测试用户"
    assert restored.cookies == mock_cookies
