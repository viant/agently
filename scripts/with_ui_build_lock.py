#!/usr/bin/env python3
"""Serialize UI dependency installation and embedding for one checkout."""
import fcntl
import hashlib
import os
import sys
import tempfile


def run(root, command):
    root = os.path.realpath(root)
    name = "agently-ui-build-" + hashlib.sha256(root.encode()).hexdigest() + ".lock"
    descriptor = os.open(os.path.join(tempfile.gettempdir(), name), os.O_CREAT | os.O_RDWR, 0o600)
    try:
        fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        print("[build-ui-embed] Waiting for the other UI build in this checkout...", flush=True)
        fcntl.flock(descriptor, fcntl.LOCK_EX)
    # Keep the advisory lock across exec; the OS releases it on process exit.
    # Do not unlink the lock file: replacing its inode would allow two owners.
    os.set_inheritable(descriptor, True)
    environment = dict(os.environ, AGENTLY_UI_BUILD_LOCK_HELD=root)
    os.execvpe(command[0], command, environment)


if __name__ == "__main__":
    if len(sys.argv) < 3:
        raise SystemExit("usage: with_ui_build_lock.py CHECKOUT COMMAND [ARG ...]")
    run(sys.argv[1], sys.argv[2:])
