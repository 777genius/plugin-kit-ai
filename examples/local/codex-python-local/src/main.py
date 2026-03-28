#!/usr/bin/env python3
import json
import sys


def read_stdin_json():
    return json.load(sys.stdin)


def handle_claude(hook_name):
    event = read_stdin_json()
    _ = event
    sys.stdout.write("{}")
    return 0


def handle_codex():
    if len(sys.argv) < 3:
        sys.stderr.write("missing notify payload\n")
        return 1
    payload = json.loads(sys.argv[2])
    _ = payload
    return 0


def main():
    if len(sys.argv) < 2:
        sys.stderr.write("usage: main.py <hook-name>\n")
        return 1

    hook_name = sys.argv[1]
    if hook_name == "notify":
        return handle_codex()
    return handle_claude(hook_name)


if __name__ == "__main__":
    raise SystemExit(main())
