"""Serve the fixtures as /api/v1 on :8766 for `EXP_MONITOR_URL=http://localhost:8766 npm run dev`.

/api/v1/campaign answers in exp-cells/1 form from cells.json (as a project's
campaign adapter prints it); legacy_campaigns.json holds the same campaigns as
the original monitor read them, for the parity tests. The fixtures are
anonymized: hosts, names and commands are generic and the metric values are
scaled by arbitrary factors, so they are not real results.
"""
import json, os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

HERE = os.path.dirname(os.path.abspath(__file__))
load = lambda n: json.load(open(os.path.join(HERE, n)))


class H(BaseHTTPRequestHandler):
    def do_GET(self):
        u = urlparse(self.path)
        q = {k: v[0] for k, v in parse_qs(u.query).items()}
        key = "%s|%s|%s" % (q.get("host"), q.get("project"), q.get("name"))
        if u.path == "/api/v1/status":
            body = load("status.json")
        elif u.path == "/api/v1/campaign":
            body = load("cells.json").get(key, {"error": "no fixture for " + key})
        elif u.path == "/api/v1/run":
            body = load("results.json").get(key, {"error": "no fixture for " + key})
        else:
            self.send_error(404)
            return
        data = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *a):
        pass


ThreadingHTTPServer(("127.0.0.1", 8766), H).serve_forever()
