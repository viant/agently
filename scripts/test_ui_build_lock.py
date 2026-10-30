import pathlib
import subprocess
import sys
import tempfile
import time
import unittest


class BuildLockTest(unittest.TestCase):
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
