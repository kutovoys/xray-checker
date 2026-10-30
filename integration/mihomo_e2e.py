#!/usr/bin/env python3
"""Exercise the Mihomo backend with real, loopback-only REALITY/h2mux traffic.

Requires Python 3, OpenSSL, a built checker, Mihomo, and a Jolymmiles Xray build
with sing-mux support. No production subscription or network access is used.
Each run generates disposable UUIDs, REALITY keys, and a cover certificate.
"""

import argparse
import http.server
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import ssl
import struct
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
import uuid


LOOPBACK = "127.0.0.1"
MUX_DESTINATION = "tcp:sp.mux.sing-box.arpa:444"


def unused_ports(count=1):
    """Find consecutive ports, needed by the checker's per-proxy listeners."""
    for _ in range(100):
        sockets = []
        try:
            first = socket.socket()
            first.bind((LOOPBACK, 0))
            sockets.append(first)
            port = first.getsockname()[1]
            if port + count > 65535:
                continue
            for offset in range(1, count):
                sock = socket.socket()
                sockets.append(sock)
                sock.bind((LOOPBACK, port + offset))
            return port
        except OSError:
            continue
        finally:
            for sock in sockets:
                sock.close()
    raise RuntimeError("Could not reserve consecutive loopback ports")


def wait_for(description, predicate, process=None, timeout=30):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if process is not None and process.poll() is not None:
            raise RuntimeError(f"Process exited while waiting for {description}")
        try:
            result = predicate()
            if result:
                return result
        except (OSError, urllib.error.URLError, json.JSONDecodeError):
            pass
        time.sleep(0.1)
    raise RuntimeError(f"Timed out waiting for {description}")


def listening(port):
    try:
        with socket.create_connection((LOOPBACK, port), timeout=0.2):
            return True
    except OSError:
        return False


def children_of(pid):
    """Linux-only process ownership assertion; does not read process arguments."""
    children = set()
    # Some container kernels omit /proc/PID/task/TID/children. PPid in status
    # is available there too, and covers children spawned by any Go OS thread.
    for path in Path("/proc").glob("[0-9]*/status"):
        try:
            for line in path.read_text().splitlines():
                if line.startswith("PPid:") and int(line.split()[1]) == pid:
                    children.add(int(path.parent.name))
                    break
        except (FileNotFoundError, ProcessLookupError, PermissionError):
            pass
    return children


def process_alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False


class Origin(http.server.BaseHTTPRequestHandler):
    status_requests = 0
    request_lock = threading.Lock()

    def do_GET(self):
        if self.path == "/ip":
            body = b"198.51.100.17"
            self.send_response(200)
        elif self.path == "/generate_204":
            with self.request_lock:
                type(self).status_requests += 1
            body = b""
            self.send_response(204)
        else:
            body = b"not found"
            self.send_response(404)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args):
        pass


class CoverTarget:
    """Small TLS 1.3 cover target accepting REALITY's PROXY-v2 prefix."""

    def __init__(self, directory):
        cert = directory / "cover.pem"
        key = directory / "cover-key.pem"
        subprocess.run(
            [
                "openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt",
                "ec_paramgen_curve:P-256", "-nodes", "-days", "1",
                "-subj", "/CN=localhost", "-addext", "subjectAltName=DNS:localhost",
                "-keyout", str(key), "-out", str(cert),
            ],
            check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        self.context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        self.context.minimum_version = ssl.TLSVersion.TLSv1_3
        self.context.load_cert_chain(cert, key)
        self.context.set_alpn_protocols(["h2", "http/1.1"])
        self.socket = socket.socket()
        self.socket.bind((LOOPBACK, 0))
        self.socket.listen()
        self.port = self.socket.getsockname()[1]
        self.closed = threading.Event()
        self.proxy_headers = 0
        self.lock = threading.Lock()
        threading.Thread(target=self.accept, daemon=True).start()

    @staticmethod
    def exact(sock, count):
        data = b""
        while len(data) < count:
            chunk = sock.recv(count - len(data))
            if not chunk:
                raise EOFError()
            data += chunk
        return data

    def handle(self, sock):
        try:
            with sock:
                sock.settimeout(4)
                header = self.exact(sock, 16)
                if header[:12] != b"\r\n\r\n\x00\r\nQUIT\n":
                    raise ValueError("Missing PROXY-v2 header")
                self.exact(sock, struct.unpack("!H", header[14:])[0])
                with self.lock:
                    self.proxy_headers += 1
                with self.context.wrap_socket(sock, server_side=True) as tls:
                    tls.recv(65536)
        except (OSError, EOFError, ValueError):
            pass

    def accept(self):
        self.socket.settimeout(0.2)
        while not self.closed.is_set():
            try:
                sock, _ = self.socket.accept()
            except socket.timeout:
                continue
            except OSError:
                break
            threading.Thread(target=self.handle, args=(sock,), daemon=True).start()

    def close(self):
        self.closed.set()
        self.socket.close()


def atomic_text(path, contents):
    temporary = path.with_suffix(path.suffix + ".new")
    temporary.write_text(contents)
    temporary.replace(path)


def subscription_text(server_port, public_key, short_id, valid_uuid, invalid_uuid,
                      max_connections=1, protocol="h2mux"):
    # Top-level traffic policy must be ignored: the checker generates its own
    # listeners and routes each listener directly to its named proxy.
    text = "mode: direct\nrules:\n  - MATCH,DIRECT\nproxies:\n"
    for name, user in [("h2mux-good", valid_uuid), ("h2mux-invalid-user", invalid_uuid)]:
        text += f"""  - name: {name}
    type: vless
    server: {LOOPBACK}
    port: {server_port}
    uuid: {user}
    network: tcp
    tls: true
    udp: true
    servername: localhost
    client-fingerprint: chrome
    reality-opts:
      public-key: {public_key}
      short-id: {json.dumps(short_id)}
      support-x25519mlkem768: true
    smux:
      enabled: true
      protocol: {protocol}
      max-connections: {max_connections}
      min-streams: 8
      padding: true
"""
    return text


def run(args, directory):
    logs = []
    processes = []
    cover = CoverTarget(directory)
    origin = http.server.ThreadingHTTPServer((LOOPBACK, 0), Origin)
    threading.Thread(target=origin.serve_forever, daemon=True).start()
    while True:
        server_port, api_port, socks_port = unused_ports(), unused_ports(), unused_ports(2)
        if len({server_port, api_port, socks_port, socks_port + 1}) == 4:
            break
    valid_uuid, invalid_uuid = str(uuid.uuid4()), str(uuid.uuid4())
    fields = {
        line.partition(":")[0].strip(): line.partition(":")[2].strip()
        for line in subprocess.check_output([args.xray, "x25519"], text=True).splitlines()
    }
    public_key = fields.get("Password (PublicKey)") or fields.get("PublicKey")
    if not public_key or not fields.get("PrivateKey"):
        raise RuntimeError("Unrecognized Xray x25519 output")
    short_id = secrets.token_hex(8)
    server_config = {
        "log": {"loglevel": "info"},
        "inbounds": [{
            "listen": LOOPBACK, "port": server_port, "protocol": "vless",
            "smux": {"h2muxMaxReadFrameSize": 16384},
            "settings": {"clients": [{"id": valid_uuid}], "decryption": "none"},
            "streamSettings": {
                "network": "raw", "security": "reality", "realitySettings": {
                    "target": f"{LOOPBACK}:{cover.port}", "xver": 2,
                    "serverNames": ["localhost"], "shortIds": [short_id],
                    "privateKey": fields["PrivateKey"],
                },
            },
        }],
        "outbounds": [{
            "protocol": "freedom",
            # The test origin is loopback. Production private-address policies
            # are not weakened by this self-contained test configuration.
            "settings": {"finalRules": [{"ip": ["127.0.0.1"], "action": "allow"}]},
        }],
    }
    config_path = directory / "server.json"
    config_path.write_text(json.dumps(server_config))
    subscription = directory / "subscription.yaml"
    atomic_text(subscription, subscription_text(
        server_port, public_key, short_id, valid_uuid, invalid_uuid,
    ))
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def api(path="/api/v1/public/proxies"):
        with opener.open(f"http://{LOOPBACK}:{api_port}{path}", timeout=2) as response:
            return json.load(response)

    def current_nodes():
        response = api()
        if response.get("success") is not True:
            return {}
        return {entry["name"]: entry for entry in response["data"]}

    def checked_pair():
        nodes = current_nodes()
        if set(nodes) != {"h2mux-good", "h2mux-invalid-user"}:
            return False
        good, bad = nodes["h2mux-good"], nodes["h2mux-invalid-user"]
        if good.get("lastCheck", 0) and bad.get("lastCheck", 0):
            if good["online"] is True and bad["online"] is False:
                return nodes
        return False

    def start(name, command):
        log = (directory / f"{name}.log").open("w")
        logs.append(log)
        proc = subprocess.Popen(command, cwd=directory, stdout=log, stderr=log)
        processes.append(proc)
        return proc

    try:
        server = start("xray", [args.xray, "run", "-config", str(config_path)])
        wait_for("REALITY listener", lambda: listening(server_port), server)
        checker = start("checker", [
            args.checker,
            "--proxy-engine=mihomo", f"--mihomo-binary={args.mihomo}",
            f"--subscription-url={subscription.as_uri()}",
            "--subscription-update-interval=2",
            f"--metrics-host={LOOPBACK}", f"--metrics-port={api_port}",
            f"--xray-start-port={socks_port}", "--proxy-check-method=status",
            f"--proxy-status-check-url=http://{LOOPBACK}:{origin.server_port}/generate_204",
            f"--proxy-ip-check-url=http://{LOOPBACK}:{origin.server_port}/ip",
            "--proxy-timeout=2", "--proxy-check-interval=1", "--log-level=info",
        ])
        initial = wait_for("successful and rejected REALITY users", checked_pair, checker)
        if Origin.status_requests == 0:
            raise AssertionError("No traffic reached the protected test origin")
        server_log = (directory / "xray.log").read_text()
        if MUX_DESTINATION not in server_log:
            raise AssertionError("No authenticated sing-mux carrier reached the server")
        if cover.proxy_headers == 0:
            raise AssertionError("REALITY did not contact the PROXY-v2 cover target")
        print("PASS: real Mihomo h2mux + VLESS/REALITY succeeds; invalid UUID remains offline")

        # Only the mux setting changes. This must rebuild the core rather than
        # treating the subscription as unchanged or dropping the setting.
        old_id = initial["h2mux-good"]["stableId"]
        atomic_text(subscription, subscription_text(
            server_port, public_key, short_id, valid_uuid, invalid_uuid, max_connections=2,
        ))

        def refreshed():
            nodes = checked_pair()
            return nodes if nodes and nodes["h2mux-good"]["stableId"] != old_id else False

        updated = wait_for("mux-only subscription refresh", refreshed, checker)
        print("PASS: changing only mux settings triggers refresh and retains correct node status")

        # This parses as a proxy but must be rejected by Mihomo configuration
        # validation. The currently running valid core must continue probing.
        previous_check = updated["h2mux-good"]["lastCheck"]
        stable_id = updated["h2mux-good"]["stableId"]
        atomic_text(subscription, subscription_text(
            server_port, public_key, short_id, valid_uuid, invalid_uuid,
            max_connections=2, protocol="not-a-mux-protocol",
        ))

        def old_config_survives():
            nodes = checked_pair()
            if not nodes:
                return False
            good = nodes["h2mux-good"]
            return (
                good["stableId"] == stable_id and
                good["lastCheck"] >= previous_check + 5
            )

        wait_for("invalid candidate rollback", old_config_survives, checker)
        print("PASS: invalid replacement does not interrupt the previous working configuration")

        child_pids = children_of(checker.pid) if sys.platform.startswith("linux") else set()
        if sys.platform.startswith("linux") and not child_pids:
            raise AssertionError("No owned Mihomo child found before shutdown")
        checker.terminate()
        checker.wait(timeout=15)
        wait_for("Mihomo listener shutdown", lambda: not listening(socks_port), timeout=10)
        if child_pids:
            wait_for("Mihomo child cleanup", lambda: all(not process_alive(pid) for pid in child_pids))
        print("PASS: checker shutdown closes its listeners and reaps the external core")
    finally:
        for proc in reversed(processes):
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
        for log in logs:
            log.close()
        origin.shutdown()
        origin.server_close()
        cover.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--checker", required=True, type=lambda value: str(Path(value).resolve()))
    parser.add_argument("--mihomo", required=True, type=lambda value: str(Path(value).resolve()))
    parser.add_argument("--xray", required=True, type=lambda value: str(Path(value).resolve()))
    parser.add_argument("--keep-workdir", action="store_true", help="Retain private test logs on completion")
    args = parser.parse_args()
    for name in ("checker", "mihomo", "xray"):
        if not Path(getattr(args, name)).is_file():
            parser.error(f"{name} executable not found")
    os.umask(0o077)
    directory = Path(tempfile.mkdtemp(prefix="checker-mihomo-e2e-"))
    try:
        run(args, directory)
    finally:
        if args.keep_workdir:
            print(f"Private test logs: {directory}")
        else:
            shutil.rmtree(directory)


if __name__ == "__main__":
    main()
