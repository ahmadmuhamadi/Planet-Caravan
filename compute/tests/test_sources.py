"""Download retries against a fake transport. No network."""

import httpx
import pytest

from app import sources


@pytest.fixture(autouse=True)
def no_sleep(monkeypatch):
    monkeypatch.setattr(sources.time, "sleep", lambda _: None)


def client(*statuses: int) -> tuple[httpx.Client, list[int]]:
    """A client answering with statuses in turn; also returns the list of calls made."""
    calls: list[int] = []

    def handle(request: httpx.Request) -> httpx.Response:
        status = statuses[len(calls)]
        calls.append(status)
        return httpx.Response(status, content=b"fits")

    return httpx.Client(transport=httpx.MockTransport(handle)), calls


def test_download_retries_server_errors():
    c, calls = client(503, 502, 200)
    assert sources.download("https://irsa/x", c) == b"fits"
    assert calls == [503, 502, 200]


def test_download_does_not_retry_client_errors():
    c, calls = client(404, 200)
    with pytest.raises(httpx.HTTPStatusError):
        sources.download("https://irsa/x", c)
    assert calls == [404]


def test_download_gives_up_after_the_last_attempt():
    c, calls = client(*[500] * sources.settings.download_attempts)
    with pytest.raises(httpx.HTTPStatusError):
        sources.download("https://irsa/x", c)
    assert len(calls) == sources.settings.download_attempts


def test_detector_number():
    assert sources.detector_number("SPHEREx-D3") == 3
