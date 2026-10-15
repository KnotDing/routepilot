"""Loopback fixture for native window checks; never contacts a real backend."""
import http.server
import json
from pathlib import Path
import sys
import time
import uuid


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def respond(self, value, content_type="application/json", cookie=None, status=200):
        data = value.encode() if isinstance(value, str) else json.dumps(value).encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        if cookie:
            self.send_header("Set-Cookie", "controller_session=" + cookie + "; Path=/; HttpOnly; SameSite=Strict")
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path == "/client.js":
            return self.respond(Path("web/src/client.js").read_text(), "text/javascript")
        if self.path == "/api/v1/session":
            time.sleep(0.08)
            cookie = self.headers.get("Cookie", "").removeprefix("controller_session=") or uuid.uuid4().hex
            return self.respond({"authenticated": True, "csrf": "csrf-" + cookie}, cookie=cookie)
        if self.path == "/":
            return self.respond("""<!doctype html><title>Isolated management page</title><input id="edit"><script type="module">
            import {request,localRequest,selectBackend,setLocalCSRF} from '/client.js';
            const selected=window.routepilotNativePreferences.managementBackend;
            selectBackend(selected?.id||'',selected?.url||'');window.routepilotActiveBackend=selected;
            window.replies=[];window.preferenceReplies=[];window.visibility=[];
            window.addEventListener('routepilot-credentials-result',e=>{window.replies.push(e.detail);if(e.detail.session)setLocalCSRF(e.detail.session.csrf)});
            window.addEventListener('routepilot-preferences-saved',e=>window.preferenceReplies.push(e.detail));
            window.addEventListener('routepilot-window-visibility',e=>window.visibility.push(e.detail.visible));
            const session=await localRequest('session');setLocalCSRF(session.csrf);
            window.write=mode=>request('mode','PUT',{mode});window.ready=true;
            </script>""", "text/html")
        self.respond({"error": "Unknown fixture route"}, status=404)

    def do_PUT(self):
        cookie = self.headers.get("Cookie", "").removeprefix("controller_session=")
        if self.headers.get("X-CSRF-Token") != "csrf-" + cookie:
            return self.respond({"error": "Fixture session/CSRF mismatch"}, status=403)
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.respond({"path": self.path, "mode": body["mode"], "session": cookie, "expected": self.headers.get("X-RoutePilot-Expected-Backend", "")})


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
Path(sys.argv[1]).write_text(str(server.server_port))
server.serve_forever()
