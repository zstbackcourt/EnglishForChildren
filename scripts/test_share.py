import importlib.util
import os
from pathlib import Path
import stat
import subprocess
import signal
import sys
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('share', Path(__file__).with_name('share.py'))
share = importlib.util.module_from_spec(spec)
spec.loader.exec_module(share)

class ShareTests(unittest.TestCase):
    def test_password_persists_privately(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'password.txt'
            password = share.load_password(path)
            self.assertGreaterEqual(len(password), 20)
            self.assertEqual(password, share.load_password(path))
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)

    def test_url_requires_exact_cloudflare_hostname(self):
        self.assertEqual(share.find_url(' | https://friendly-tree.trycloudflare.com |'),
                         'https://friendly-tree.trycloudflare.com')
        for text in ['https://trycloudflare.com', 'https://a.trycloudflare.com.evil.test',
                     'https://a.example.com', 'http://a.trycloudflare.com']:
            self.assertIsNone(share.find_url(text))

    @unittest.skipUnless(hasattr(signal, 'SIGHUP'), 'Unix terminal signal')
    def test_terminal_close_requests_cleanup(self):
        code = """
import sys, threading
sys.path.insert(0, sys.argv[1])
import share
stop = threading.Event()
share.install_stop_signals(stop)
print('ready', flush=True)
stop.wait(5)
print('cleanup' if stop.is_set() else 'timeout', flush=True)
"""
        process = subprocess.Popen([sys.executable, '-c', code, str(Path(__file__).parent)],
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   universal_newlines=True)
        try:
            self.assertEqual(process.stdout.readline().strip(), 'ready')
            process.send_signal(signal.SIGHUP)
            out, error = process.communicate(timeout=5)
            self.assertEqual(process.returncode, 0, error)
            self.assertIn('cleanup', out)
        finally:
            if process.poll() is None:
                process.kill()
            process.communicate()

    def test_stop_reaps_its_child(self):
        child = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(30)'],
                                 start_new_session=True)
        share.terminate(child)
        self.assertIsNotNone(child.poll())
        share.terminate(child)  # Stopping an already stopped child is harmless.

    def test_existing_blank_password_is_not_used(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'password.txt'
            path.write_text('  \n')
            with self.assertRaises(ValueError):
                share.load_password(path)

if __name__ == '__main__':
    unittest.main()
