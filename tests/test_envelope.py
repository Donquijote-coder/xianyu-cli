"""Tests for the output envelope."""

import json

from xianyu_cli.models.envelope import Envelope, fail, ok


def test_ok_envelope():
    env = ok({"key": "value"})
    assert env.ok is True
    assert env.data == {"key": "value"}
    assert env.error is None


def test_fail_envelope():
    env = fail("something went wrong")
    assert env.ok is False
    assert env.error == "something went wrong"
    assert env.data is None


def test_envelope_to_json():
    env = ok({"items": [1, 2, 3]})
    j = json.loads(env.to_json())
    assert j["ok"] is True
    assert j["data"]["items"] == [1, 2, 3]
    assert j["schema_version"] == "1.0.0"


def test_envelope_to_dict():
    env = fail("error")
    d = env.to_dict()
    assert d["ok"] is False
    assert d["error"] == "error"
    assert "schema_version" in d
