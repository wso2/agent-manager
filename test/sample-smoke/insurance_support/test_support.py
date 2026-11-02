"""Regression coverage for process cleanup after a termination timeout."""
import signal
import subprocess
import unittest
from unittest.mock import Mock, call, patch

from support import stop_process


class ProcessCleanupTests(unittest.TestCase):
    def test_kills_and_reaps_process_when_sigterm_times_out(self):
        process = Mock(pid=12345)
        process.poll.return_value = None
        process.wait.side_effect = [subprocess.TimeoutExpired("sample", 10), 0]

        with patch("support.os.killpg") as killpg:
            stop_process(process)

        self.assertEqual(killpg.call_args_list, [
            call(process.pid, signal.SIGTERM),
            call(process.pid, signal.SIGKILL),
        ])
        self.assertEqual(process.wait.call_args_list, [call(timeout=10), call(timeout=5)])
