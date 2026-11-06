import json
import pathlib
import os
import shlex
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest


class BuildLockTest(unittest.TestCase):
    def test_symlink_checkout_acquires_once_before_build_tools(self):
        repository = pathlib.Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory() as directory:
            base = pathlib.Path(directory)
            root, alias, commands = base / "checkout", base / "alias", base / "commands"
            (root / "e2e").mkdir(parents=True)
            (root / "scripts").mkdir()
            commands.mkdir()
            alias.symlink_to(root, target_is_directory=True)
            for relative in ("e2e/build-ui-embed.sh", "scripts/with_ui_build_lock.py"):
                shutil.copyfile(repository / relative, root / relative)
            attempts, reached, installed = [base / name for name in ("attempts", "reached", "installed")]
            shims = {
                "python3": '#!/bin/sh\nprintf "attempt\\n" >> "$BUILD_TEST_ATTEMPTS"\nexec ' + shlex.quote(sys.executable) + ' "$@"\n',
                "node": '#!/bin/sh\n[ "$AGENTLY_UI_BUILD_LOCK_HELD" = "$BUILD_TEST_ROOT" ] || exit 39\nprintf reached > "$BUILD_TEST_REACHED"\nexit 37\n',
                "npm": '#!/bin/sh\nprintf unexpected > "$BUILD_TEST_INSTALLED"\nexit 38\n',
            }
            for name, content in shims.items():
                path = commands / name
                path.write_text(content)
                path.chmod(0o700)
            environment = dict(os.environ, PATH=str(commands) + os.pathsep + os.environ.get("PATH", ""),
                BUILD_TEST_ATTEMPTS=str(attempts), BUILD_TEST_ROOT=str(root.resolve()),
                BUILD_TEST_REACHED=str(reached), BUILD_TEST_INSTALLED=str(installed))
            environment.pop("AGENTLY_UI_BUILD_LOCK_HELD", None)
            process = subprocess.Popen(["bash", str(alias / "e2e/build-ui-embed.sh")], env=environment,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
            try:
                stdout, stderr = process.communicate(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.communicate()
                self.fail("symlink checkout waited on its own build lock")
            self.assertEqual(process.returncode, 37, stderr)
            self.assertEqual(attempts.read_text().splitlines(), ["attempt"])
            self.assertTrue(reached.exists())
            self.assertFalse(installed.exists(), "the regression must not install packages")
            self.assertNotIn("Waiting", stdout)

    def test_forge_install_follows_file_or_published_dependency(self):
        repository = pathlib.Path(__file__).resolve().parent.parent
        for dependency in ("file:../../forge", "https://codeload.github.com/viant/forge/tar.gz/published"):
            with self.subTest(dependency=dependency), tempfile.TemporaryDirectory() as directory:
                base = pathlib.Path(directory).resolve()
                root, commands = base / "checkout", base / "commands"
                (root / "e2e").mkdir(parents=True)
                (root / "ui").mkdir()
                sdk, forge = base / "sdk", base / "forge"
                sdk.mkdir()
                forge.mkdir()
                commands.mkdir()
                shutil.copyfile(repository / "e2e/build-ui-embed.sh", root / "e2e/build-ui-embed.sh")
                (root / "ui/package.json").write_text(json.dumps({"dependencies": {
                    "agently-core-ui-sdk": "file:../../sdk", "forge": dependency}}))
                for package in (sdk, forge):
                    (package / "package.json").write_text("{}")
                    (package / "package-lock.json").write_text("{}")
                calls = base / "npm-calls"
                npm = commands / "npm"
                npm.write_text('#!/bin/sh\nprintf "%s %s\\n" "$PWD" "$*" >> "$BUILD_TEST_CALLS"\n[ "$1" != run ] || exit 37\n')
                npm.chmod(0o700)
                result = subprocess.run(["bash", str(root / "e2e/build-ui-embed.sh")],
                    env=dict(os.environ, PATH=str(commands) + os.pathsep + os.environ.get("PATH", ""),
                        AGENTLY_UI_BUILD_LOCK_HELD=str(root), BUILD_TEST_CALLS=str(calls)),
                    capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, 37, result.stderr)
                expected = []
                if dependency.startswith("file:"):
                    expected.append(str(forge) + " ci --include=dev")
                expected.extend([str(sdk) + " ci --include=dev", str(root / "ui") + " install --include=dev",
                    str(root / "ui") + " run build"])
                self.assertEqual(calls.read_text().splitlines(), expected)

    def test_two_builds_are_serial_and_exit_releases_the_lock(self):
        helper = pathlib.Path(__file__).with_name("with_ui_build_lock.py")
        with tempfile.TemporaryDirectory() as root:
            ready, release, second = [pathlib.Path(root) / name for name in ("ready", "release", "second")]
            holder = "import pathlib,time,sys; r=pathlib.Path(sys.argv[1]); (r/'ready').touch(); " \
                     "\nwhile not (r/'release').exists(): time.sleep(.01)"
            first = subprocess.Popen([sys.executable, str(helper), root, sys.executable, "-c", holder, root])
            follower = None
            try:
                deadline = time.monotonic() + 10
                while not ready.exists() and time.monotonic() < deadline:
                    time.sleep(.01)
                self.assertTrue(ready.exists())
                follower = subprocess.Popen([sys.executable, str(helper), root, sys.executable, "-c",
                    "import pathlib,sys; pathlib.Path(sys.argv[1]).touch()", str(second)], stdout=subprocess.PIPE, text=True)
                self.assertIn("Waiting", follower.stdout.readline())
                self.assertFalse(second.exists())
                release.touch()
                self.assertEqual(first.wait(timeout=10), 0)
                self.assertEqual(follower.wait(timeout=10), 0)
                self.assertTrue(second.exists())
            finally:
                for process in (first, follower):
                    if process and process.poll() is None:
                        process.kill(); process.wait()
                if follower and follower.stdout:
                    follower.stdout.close()


if __name__ == "__main__":
    unittest.main()
