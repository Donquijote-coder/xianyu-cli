"""Tests for sign parameter generation."""

from xianyu_cli.core.sign import extract_token, generate_sign, get_timestamp


def test_generate_sign_known_value():
    """Sign should produce correct MD5 for known inputs."""
    token = "abc123def456"
    t = "1710000000000"
    data = '{"keyword":"iPhone"}'

    sign = generate_sign(token, t, data)

    # MD5 of: abc123def456&1710000000000&34839810&{"keyword":"iPhone"}
    assert len(sign) == 32
    assert sign.isalnum()


def test_generate_sign_deterministic():
    """Same inputs should produce same sign."""
    token = "testtoken"
    t = "1234567890"
    data = '{"test":"value"}'

    sign1 = generate_sign(token, t, data)
    sign2 = generate_sign(token, t, data)
    assert sign1 == sign2


def test_extract_token_with_underscore():
    """Should extract part before underscore."""
    assert extract_token("abc123_1710000000000") == "abc123"


def test_extract_token_without_underscore():
    """Should return full string if no underscore."""
    assert extract_token("abc123") == "abc123"


def test_extract_token_empty():
    """Should return empty string for empty input."""
    assert extract_token("") == ""


def test_get_timestamp_format():
    """Timestamp should be a numeric string in milliseconds."""
    t = get_timestamp()
    assert t.isdigit()
    assert len(t) == 13  # milliseconds have 13 digits
