#!/usr/bin/env python3
"""Exercise the legacy Alertmanager v1 API through one or more targets.

This is a dependency-free Python 3 client. It covers the client classes named in
the story 025 inventory: alert sender, alert reader/filterer, silence manager,
status/receiver reader and versionInfo consumer. Run it against a proxy, the
v0.26 reference, or a production endpoint once its auth/path details are known.

Examples:
  python3 test/clients/v1_client.py --url http://127.0.0.1:19391
  python3 test/clients/v1_client.py --url http://127.0.0.1:19391 \
    --url http://127.0.0.1:19392 --url http://127.0.0.1:19393 \
    --receiver http://127.0.0.1:19280
  python3 test/clients/v1_client.py --url https://am.example/proxy \
    --prefix /api --header 'Authorization: Bearer ...' --insecure

Writes go to the first --url; every --url is checked for silence gossip when
more than one is given.
"""
from __future__ import annotations

import argparse
import json
import re
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from datetime import datetime, timedelta, timezone


class ProbeError(RuntimeError):
    pass


def check(condition: bool, message: str) -> None:
    if not condition:
        raise ProbeError(message)


def utcnow() -> datetime:
    return datetime.now(timezone.utc)


def stamp(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def parse_stamp(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


class Client:
    def __init__(self, base: str, prefix: str, headers: dict[str, str], timeout: float, insecure: bool):
        self.base = base.rstrip("/")
        self.prefix = "/" + prefix.strip("/") if prefix.strip("/") else ""
        self.headers = headers
        self.timeout = timeout
        if insecure:
            context = ssl._create_unverified_context()
            self.opener = urllib.request.build_opener(urllib.request.HTTPSHandler(context=context))
        else:
            self.opener = urllib.request.build_opener()

    def request(self, method: str, path: str, body: object | None = None) -> tuple[int, dict[str, str], bytes]:
        data = None
        headers = {"Accept": "application/json", **self.headers}
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(self.base + self.prefix + path, data=data, method=method, headers=headers)
        try:
            with self.opener.open(request, timeout=self.timeout) as response:
                return response.status, dict(response.headers), response.read()
        except urllib.error.HTTPError as error:
            return error.code, dict(error.headers), error.read()

    def json_request(self, method: str, path: str, body: object | None = None, expect: int = 200) -> object:
        status, headers, raw = self.request(method, path, body)
        text = raw.decode("utf-8", "replace")
        if status != expect:
            raise ProbeError(f"{method} {path}: status {status}, want {expect}: {text[:300]}")
        if not text:
            return None
        try:
            return json.loads(text)
        except json.JSONDecodeError as error:
            raise ProbeError(f"{method} {path}: invalid JSON: {error}: {text[:300]}") from error


def wait_for(description: str, predicate, timeout: float = 15.0):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            value = predicate()
            if value:
                return value
        except ProbeError as error:
            last = error
        time.sleep(0.25)
    raise ProbeError(f"timed out waiting for {description}: {last}")


def alert_query(run_id: str, alertname: str | None = None) -> str:
    matchers = f'v1_python_run="{run_id}"'
    if alertname:
        matchers += f',alertname="{alertname}"'
    return urllib.parse.urlencode({"filter": "{" + matchers + "}"})


def first_alert(alerts: object) -> dict:
    check(isinstance(alerts, dict) and alerts.get("status") == "success", "alert response is not a v1 success envelope")
    data = alerts.get("data")
    check(isinstance(data, list), "alert response data is not a list")
    check(len(data) == 1, f"expected one matching alert, got {len(data)}")
    return data[0]


def assert_error(client: Client, method: str, path: str, status: int, contains: str | None = None) -> None:
    code, headers, raw = client.request(method, path)
    text = raw.decode("utf-8", "replace")
    check(code == status, f"{method} {path}: status {code}, want {status}: {text[:200]}")
    if contains is not None:
        check(contains in text, f"{method} {path}: response does not contain {contains!r}: {text[:200]}")


def run(args: argparse.Namespace) -> dict:
    headers = {}
    for item in args.header:
        name, separator, value = item.partition(":")
        if not separator:
            raise ProbeError(f"--header must be 'Name: value', got {item!r}")
        headers[name.strip()] = value.strip()
    clients = [Client(url, args.prefix, headers, args.timeout, args.insecure) for url in args.url]
    primary = clients[0]
    run_id = args.run_id or f"py-v1-{uuid.uuid4().hex[:12]}"
    checks: list[str] = []

    def ok(name: str) -> None:
        checks.append(name)
        if args.verbose:
            print(f"ok {name}")

    now = utcnow()
    alert_name = "PythonV1Client"
    alert_labels = {"alertname": alert_name, "v1_python_run": run_id, "source": "python-v1-client"}
    alert_annotations = {"summary": "python v1 client probe"}
    start = now - timedelta(minutes=1)
    end = now + timedelta(minutes=10)
    generator_url = "http://python-v1-client/graph?g0"

    # Status + receiver readers, including versionInfo.
    status = primary.json_request("GET", "/api/v1/status")
    check(isinstance(status, dict) and status.get("status") == "success", "v1 status is not a success envelope")
    data = status.get("data") or {}
    for field in ("configYAML", "configJSON", "versionInfo", "uptime", "clusterStatus"):
        check(field in data, f"v1 status is missing {field}")
    version = (data.get("versionInfo") or {}).get("version")
    check(bool(version), "v1 status versionInfo.version is empty")
    ok("status/config/versionInfo")

    receivers = primary.json_request("GET", "/api/v1/receivers")
    check(isinstance(receivers, dict) and receivers.get("status") == "success", "v1 receivers is not a success envelope")
    receiver_names = receivers.get("data")
    check(isinstance(receiver_names, list) and receiver_names and all(isinstance(r, str) for r in receiver_names),
          "v1 receivers data is not a non-empty list of names")
    ok("receivers")

    # Recorded error and routing contract.
    assert_error(primary, "GET", "/api/v1/alerts?filter=%7Bbad", 400, "bad matcher format")
    assert_error(primary, "GET", "/api/v1/alerts?silenced=maybe", 400, "can either be 'true' or 'false'")
    assert_error(primary, "GET", "/api/v1/nope", 404, "404 page not found")
    assert_error(primary, "PUT", "/api/v1/alerts", 405, "Method Not Allowed")
    code, response_headers, _ = primary.request("OPTIONS", "/api/v1/alerts")
    check(code == 200 and response_headers.get("Access-Control-Allow-Origin") == "*", "v1 preflight contract changed")
    ok("error/preflight contract")

    # Alert sender + braced/unbraced readers.
    firing = [{
        "labels": alert_labels,
        "annotations": alert_annotations,
        "startsAt": stamp(start),
        "endsAt": stamp(end),
        "generatorURL": generator_url,
    }]
    primary.json_request("POST", "/api/v1/alerts", firing)
    alert = first_alert(primary.json_request("GET", "/api/v1/alerts?" + alert_query(run_id, alert_name)))
    check(alert["labels"].get("alertname") == alert_name and alert["labels"].get("v1_python_run") == run_id,
          f"posted alert labels changed: {alert.get('labels')}")
    check(alert.get("annotations") == alert_annotations, f"posted alert annotations changed: {alert.get('annotations')}")
    check(alert.get("generatorURL") == generator_url, f"generatorURL changed: {alert.get('generatorURL')}")
    check(abs((parse_stamp(alert["startsAt"]) - start).total_seconds()) < 2, "startsAt changed by more than 2s")
    check(abs((parse_stamp(alert["endsAt"]) - end).total_seconds()) < 2, "endsAt changed by more than 2s")
    check(isinstance(alert.get("receivers"), list) and all(isinstance(r, str) for r in alert["receivers"]),
          "v1 alert receivers are not strings")
    unbraced = first_alert(primary.json_request("GET", "/api/v1/alerts?filter=" +
                                                urllib.parse.quote(f'v1_python_run="{run_id}",alertname="{alert_name}"')))
    check(unbraced["fingerprint"] == alert["fingerprint"], "unbraced filter returned a different alert")
    receiver_query = urllib.parse.quote(re.escape(alert["receivers"][0]))
    filtered = first_alert(primary.json_request("GET", f"/api/v1/alerts?receiver={receiver_query}&{alert_query(run_id, alert_name)}"))
    check(filtered["fingerprint"] == alert["fingerprint"], "receiver regex filter lost the alert")
    not_silenced = primary.json_request("GET", "/api/v1/alerts?silenced=false&" + alert_query(run_id, alert_name))
    check(len(not_silenced.get("data", [])) == 1, "unsilenced alert missing before silence")
    ok("alert sender/filter/receiver/timestamps")

    # Resolved alert is accepted but not listed as active.
    resolved = [{
        "labels": {"alertname": alert_name, "v1_python_run": run_id, "resolved": "true"},
        "annotations": {"summary": "resolved python v1 probe"},
        "startsAt": stamp(now - timedelta(minutes=10)),
        "endsAt": stamp(now - timedelta(minutes=5)),
    }]
    primary.json_request("POST", "/api/v1/alerts", resolved)
    after_resolved = primary.json_request("GET", "/api/v1/alerts?" + alert_query(run_id, alert_name))
    check(len(after_resolved.get("data", [])) == 1, "resolved alert was listed as active")
    ok("resolved alert accepted and excluded")

    active_silences: list[str] = []
    old_silence: str | None = None
    try:
        silence_body = {
            "matchers": [{"name": "v1_python_run", "value": run_id, "isRegex": False, "isEqual": True}],
            "startsAt": stamp(now),
            "endsAt": stamp(now + timedelta(minutes=10)),
            "createdBy": "python-v1-client",
            "comment": "python v1 client probe",
        }
        created = primary.json_request("POST", "/api/v1/silences", silence_body)
        silence_id = (created.get("data") or {}).get("silenceId")
        check(bool(silence_id), f"v1 create silence returned no silenceId: {created}")
        active_silences.append(silence_id)
        ok("silence create")

        listed = primary.json_request("GET", "/api/v1/silences?filter=" +
                                      urllib.parse.quote(f'{{v1_python_run="{run_id}"}}'))
        listed_ids = [s.get("id") for s in listed.get("data", [])]
        check(silence_id in listed_ids, "created silence missing from filtered list")
        got = (primary.json_request("GET", f"/api/v1/silence/{silence_id}") or {}).get("data") or {}
        check(got.get("id") == silence_id and got.get("status", {}).get("state") == "active",
              f"get silence returned {got}")
        check(any(m.get("name") == "v1_python_run" and m.get("value") == run_id for m in got.get("matchers", [])),
              f"silence matchers changed: {got.get('matchers')}")
        ok("silence list/get/filter")

        def suppressed() -> bool:
            alerts = primary.json_request("GET", "/api/v1/alerts?" + alert_query(run_id, alert_name))
            data = alerts.get("data") or []
            if len(data) != 1:
                return False
            status_block = data[0].get("status") or {}
            return status_block.get("state") == "suppressed" and silence_id in (status_block.get("silencedBy") or [])

        wait_for("alert suppression", suppressed)
        if args.verbose:
            print("ok alert suppressed by silence")
        checks.append("alert suppression")
        excluded = primary.json_request("GET", "/api/v1/alerts?silenced=false&" + alert_query(run_id, alert_name))
        check(len(excluded.get("data", [])) == 0, "silenced=false still returned the silenced alert")
        ok("silenced=false excludes alert")

        stored_start = got["startsAt"]
        extended = primary.json_request("POST", "/api/v1/silences", {
            **silence_body,
            "id": silence_id,
            "startsAt": stored_start,
            "endsAt": stamp(now + timedelta(minutes=15)),
            "comment": "python v1 client extended",
        })
        check((extended.get("data") or {}).get("silenceId") == silence_id, "in-place silence extension changed the ID")
        got_extended = (primary.json_request("GET", f"/api/v1/silence/{silence_id}") or {}).get("data") or {}
        check(parse_stamp(got_extended["endsAt"]) > parse_stamp(got["endsAt"]), "silence extension did not move endsAt")
        ok("silence in-place extension")

        changed = primary.json_request("POST", "/api/v1/silences", {
            **silence_body,
            "id": silence_id,
            "matchers": [
                {"name": "v1_python_run", "value": run_id, "isRegex": False, "isEqual": True},
                {"name": "alertname", "value": alert_name, "isRegex": False, "isEqual": True},
            ],
            "startsAt": stored_start,
            "endsAt": stamp(now + timedelta(minutes=20)),
            "comment": "python v1 client narrowed",
        })
        new_id = (changed.get("data") or {}).get("silenceId")
        check(bool(new_id) and new_id != silence_id, "changing matchers did not create a new silence ID")
        active_silences.append(new_id)
        old = (primary.json_request("GET", f"/api/v1/silence/{silence_id}") or {}).get("data") or {}
        check(old.get("status", {}).get("state") == "expired", "old silence did not expire after matcher change")
        ok("silence matcher change creates new ID")

        if len(clients) > 1:
            for index, client in enumerate(clients[1:], start=2):
                wait_for(f"silence gossip to target {index}", lambda c=client: (
                    c.request("GET", f"/api/v1/silence/{new_id}")[0] == 200
                ))
            ok(f"silence gossip to {len(clients)} targets")

        muting_checked = False
        if args.receiver and not args.no_muting:
            receiver = Client(args.receiver, "", headers, args.timeout, args.insecure)
            clear_status, _, _ = receiver.request("DELETE", "/notifications")
            if clear_status == 204:
                mute = [{
                    "labels": {"alertname": "PythonV1MuteProbe", "v1_python_run": run_id, "mute": "true"},
                    "annotations": {"summary": "must stay muted"},
                    "startsAt": stamp(utcnow() - timedelta(minutes=1)),
                    "endsAt": stamp(utcnow() + timedelta(minutes=5)),
                }]
                primary.json_request("POST", "/api/v1/alerts", mute)
                time.sleep(5)
                _, _, raw = receiver.request("GET", "/notifications")
                notifications = json.loads(raw.decode() or "[]")
                for notification in notifications:
                    for item in notification.get("alerts", []):
                        labels = item.get("labels") or {}
                        check(labels.get("v1_python_run") != run_id,
                              f"silenced alert was delivered to the receiver: {labels}")
                muting_checked = True
                ok("silenced alert produced no receiver notification")
            else:
                checks.append("receiver muting skipped (no notification clear endpoint)")
        elif args.receiver:
            checks.append("receiver muting disabled")

        # Delete the active silence and verify the alert becomes active again.
        primary.json_request("DELETE", f"/api/v1/silence/{new_id}")
        deleted = (primary.json_request("GET", f"/api/v1/silence/{new_id}") or {}).get("data") or {}
        check(deleted.get("status", {}).get("state") == "expired", "deleted silence is not expired")
        primary.json_request("DELETE", f"/api/v1/silence/{new_id}")
        assert_error(primary, "DELETE", "/api/v1/silence/00000000-0000-0000-0000-000000000000", 400, "silence not found")
        ok("silence delete/delete-again/unknown")

        def active_again() -> bool:
            alerts = primary.json_request("GET", "/api/v1/alerts?" + alert_query(run_id, alert_name))
            data = alerts.get("data") or []
            return len(data) == 1 and (data[0].get("status") or {}).get("state") == "active"

        wait_for("alert active after silence deletion", active_again)
        ok("alert active after silence deletion")
    finally:
        if not args.keep:
            for silence_id in active_silences:
                try:
                    primary.request("DELETE", f"/api/v1/silence/{silence_id}")
                except Exception:
                    pass

    return {
        "ok": True,
        "version": version,
        "urls": args.url,
        "run_id": run_id,
        "checks": checks,
        "muting": muting_checked,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--url", action="append", required=True,
                        help="proxy/reference base URL; repeat to verify silence gossip")
    parser.add_argument("--prefix", default="", help="path prefix prepended to every API path")
    parser.add_argument("--header", action="append", default=[], help="extra 'Name: value' request header")
    parser.add_argument("--receiver", help="lab receiver base URL (enables the muting check)")
    parser.add_argument("--no-muting", action="store_true", help="skip the receiver muting check")
    parser.add_argument("--timeout", type=float, default=10.0)
    parser.add_argument("--insecure", action="store_true", help="skip TLS certificate verification")
    parser.add_argument("--run-id", help="unique probe label value (default: random)")
    parser.add_argument("--json-out", help="write the summary JSON here")
    parser.add_argument("--keep", action="store_true", help="leave probe silences behind on failure")
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()
    started = time.monotonic()
    try:
        summary = run(args)
        summary["duration_seconds"] = round(time.monotonic() - started, 3)
        print(json.dumps(summary, indent=2))
        if args.json_out:
            with open(args.json_out, "w", encoding="utf-8") as output:
                json.dump(summary, output, indent=2)
                output.write("\n")
        return 0
    except (ProbeError, OSError, ValueError, KeyError, TypeError) as error:
        print(f"v1 client probe FAILED: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
