#!/usr/bin/env python3
"""Minimal webhook receiver for cub-scout watch examples."""

import argparse
import datetime
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Local webhook receiver for cub-scout watch events")
    parser.add_argument("--host", default="127.0.0.1", help="Bind host (default: 127.0.0.1)")
    parser.add_argument("--port", type=int, default=8787, help="Bind port (default: 8787)")
    parser.add_argument(
        "--output",
        default="/tmp/cub-scout-watch-events.jsonl",
        help="Output JSONL file path (default: /tmp/cub-scout-watch-events.jsonl)",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:  # noqa: N802
            content_length = int(self.headers.get("Content-Length", "0"))
            raw_body = self.rfile.read(content_length)

            try:
                event = json.loads(raw_body.decode("utf-8"))
            except json.JSONDecodeError:
                self.send_response(400)
                self.end_headers()
                self.wfile.write(b"invalid json")
                return

            envelope = {
                "receivedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                "path": self.path,
                "event": event,
            }

            with output_path.open("a", encoding="utf-8") as f:
                f.write(json.dumps(envelope, separators=(",", ":")) + "\n")

            event_type = event.get("type", "unknown")
            print(f"[{datetime.datetime.now().isoformat(timespec='seconds')}] event={event_type} path={self.path}")

            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok")

        def log_message(self, format: str, *args) -> None:  # noqa: A003
            # Keep output focused on event summaries above.
            return

    server = HTTPServer((args.host, args.port), Handler)
    print(f"Listening on http://{args.host}:{args.port} -> {output_path}")
    print("Press Ctrl+C to stop")

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
