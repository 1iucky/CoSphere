import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SLEEP_SECONDS = 12

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get('Content-Length', 0))
        if length > 0:
            _ = self.rfile.read(length)
        time.sleep(SLEEP_SECONDS)
        body = {
            "id": "mock",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": "glm-4.7",
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": "ok"},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
        }
        data = json.dumps(body).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, format, *args):
        return


def main():
    server = ThreadingHTTPServer(("127.0.0.1", 18000), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
