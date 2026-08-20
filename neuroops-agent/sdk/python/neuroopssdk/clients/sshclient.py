#  NeurOps Agent — Python SDK SSH Client
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  Wraps Paramiko for metric and runbook plugins that collect data via SSH.
#  Supports password auth, key-based auth, SFTP file transfer, and
#  interactive (pty) sessions for multi-step CLI interactions.
#
#  Usage:
#    client = SSHClient(context, logger)
#    result = client.connect()
#    if result["status"] == "succeed":
#        out = client.execute("df -h")
#        client.disconnect()

import io
import time

import paramiko

from neuroopssdk.constants import Constant


class SSHClient:

    DEFAULT_PORT    = 22
    DEFAULT_TIMEOUT = 30

    def __init__(self, context: dict, logger):
        self._log      = logger
        self._host     = context.get(Constant.OBJECT_IP, "")
        self._port     = int(context.get("ssh.port", context.get(Constant.PORT, self.DEFAULT_PORT)))
        self._username = context.get(Constant.USERNAME, "")
        self._password = context.get(Constant.PASSWORD, "")
        self._key      = context.get("ssh.key", "")
        self._passphrase = context.get("ssh.passphrase", "")
        self._timeout  = int(context.get(Constant.TIMEOUT, self.DEFAULT_TIMEOUT))
        self._client: paramiko.SSHClient | None = None

    # ── Lifecycle ──────────────────────────────────────────────────────────

    def connect(self) -> dict:
        try:
            self._client = paramiko.SSHClient()
            self._client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

            kwargs = dict(
                hostname=self._host,
                port=self._port,
                username=self._username,
                timeout=self._timeout,
                look_for_keys=False,
                allow_agent=False,
            )

            if self._key:
                pkey = self._load_key(self._key, self._passphrase)
                if pkey is None:
                    return self._fail(
                        f"Failed to load SSH private key for {self._host}",
                        Constant.ERROR_INVALID_CREDENTIALS,
                    )
                kwargs["pkey"] = pkey
            else:
                kwargs["password"] = self._password

            self._client.connect(**kwargs)
            self._log.infof(Constant.INFO_MSG_CONNECTED, self._host, self._port)
            return {"status": Constant.STATUS_SUCCEED}

        except paramiko.AuthenticationException:
            return self._fail(
                f"Authentication Error: Invalid credentials for {self._host}:{self._port}",
                Constant.ERROR_INVALID_CREDENTIALS,
            )
        except paramiko.ssh_exception.NoValidConnectionsError:
            return self._fail(
                f"Connection Error: Could not connect to {self._host}:{self._port}",
                Constant.ERROR_CONNECTION_FAILED,
            )
        except TimeoutError:
            return self._fail(
                f"Timeout Error: SSH connection to {self._host}:{self._port} timed out",
                Constant.ERROR_TIMEOUT,
            )
        except Exception as e:
            return self._fail(str(e), Constant.ERROR_INTERNAL)

    def disconnect(self):
        if self._client:
            try:
                self._client.close()
            except Exception:
                pass
            self._client = None
            self._log.infof("Disconnected from %s:%d", self._host, self._port)

    # ── Command execution ──────────────────────────────────────────────────

    def execute(self, command: str, timeout: int = None) -> dict:
        """
        Run a single command and return stdout/stderr.
        Returns: {"status": "succeed", "output": "...", "error_output": "..."}
        """
        if not self._client:
            return self._fail("Not connected", Constant.ERROR_CONNECTION_FAILED)
        try:
            t = timeout or self._timeout
            _, stdout, stderr = self._client.exec_command(command, timeout=t)
            out = stdout.read().decode(errors="replace").strip()
            err = stderr.read().decode(errors="replace").strip()
            self._log.tracef("SSH command: %s → %s", command, out[:200])
            return {
                "status":       Constant.STATUS_SUCCEED,
                "output":       out,
                "error_output": err,
            }
        except Exception as e:
            return self._fail(
                f"Command execution failed [{command}]: {e}",
                Constant.ERROR_COMMAND_EXECUTION_FAILED,
            )

    def execute_many(self, commands: list) -> dict:
        """
        Execute multiple commands sequentially.
        Returns: {"status": "succeed", "results": [<per-cmd result>, ...]}
        """
        results = []
        for cmd in commands:
            r = self.execute(cmd)
            results.append({"command": cmd, **r})
            if r["status"] != Constant.STATUS_SUCCEED:
                break
        return {
            "status":  Constant.STATUS_SUCCEED,
            "results": results,
        }

    # ── Interactive (PTY) session ──────────────────────────────────────────

    def open_shell(self, width: int = 200, height: int = 50) -> paramiko.Channel:
        """Open an interactive PTY channel (for multi-step CLI workflows)."""
        if not self._client:
            raise RuntimeError("Not connected")
        channel = self._client.invoke_shell(width=width, height=height)
        channel.settimeout(self._timeout)
        time.sleep(1)
        self._flush(channel)
        return channel

    def send_command(self, channel: paramiko.Channel, command: str,
                     wait_pattern: str = None, delay: float = 1.0) -> str:
        """
        Send a command on an open shell channel and collect output.
        Optionally waits until wait_pattern appears in the output.
        """
        channel.send(command + "\n")
        time.sleep(delay)
        return self._read_until(channel, wait_pattern, timeout=self._timeout)

    # ── SFTP ──────────────────────────────────────────────────────────────

    def upload(self, local_path: str, remote_path: str) -> dict:
        if not self._client:
            return self._fail("Not connected", Constant.ERROR_CONNECTION_FAILED)
        try:
            sftp = self._client.open_sftp()
            sftp.put(local_path, remote_path)
            sftp.close()
            return {"status": Constant.STATUS_SUCCEED}
        except Exception as e:
            return self._fail(f"SFTP upload failed: {e}", Constant.ERROR_INTERNAL)

    def download(self, remote_path: str, local_path: str) -> dict:
        if not self._client:
            return self._fail("Not connected", Constant.ERROR_CONNECTION_FAILED)
        try:
            sftp = self._client.open_sftp()
            sftp.get(remote_path, local_path)
            sftp.close()
            return {"status": Constant.STATUS_SUCCEED}
        except Exception as e:
            return self._fail(f"SFTP download failed: {e}", Constant.ERROR_INTERNAL)

    def read_remote_file(self, remote_path: str) -> dict:
        """Read a remote file and return its contents as a string."""
        if not self._client:
            return self._fail("Not connected", Constant.ERROR_CONNECTION_FAILED)
        try:
            sftp = self._client.open_sftp()
            with sftp.open(remote_path, "r") as f:
                content = f.read().decode(errors="replace")
            sftp.close()
            return {"status": Constant.STATUS_SUCCEED, "content": content}
        except Exception as e:
            return self._fail(f"Failed to read {remote_path}: {e}", Constant.ERROR_INTERNAL)

    # ── Helpers ────────────────────────────────────────────────────────────

    def _load_key(self, key_data: str, passphrase: str = None):
        passphrase = passphrase.encode() if passphrase else None
        for key_class in (
            paramiko.RSAKey,
            paramiko.Ed25519Key,
            paramiko.ECDSAKey,
            paramiko.DSSKey,
        ):
            try:
                return key_class.from_private_key(io.StringIO(key_data), password=passphrase)
            except Exception:
                continue
        return None

    def _flush(self, channel: paramiko.Channel) -> str:
        buf = b""
        while channel.recv_ready():
            buf += channel.recv(4096)
        return buf.decode(errors="replace")

    def _read_until(self, channel: paramiko.Channel,
                    pattern: str = None, timeout: float = 30) -> str:
        import time as _time
        buf = ""
        start = _time.time()
        while True:
            if channel.recv_ready():
                buf += channel.recv(4096).decode(errors="replace")
                if pattern and pattern in buf:
                    break
            elif _time.time() - start > timeout:
                break
            else:
                _time.sleep(0.1)
        return buf

    def _fail(self, message: str, code: str = Constant.ERROR_INTERNAL) -> dict:
        self._log.error(message)
        return {
            "status":     Constant.STATUS_FAIL,
            "error":      message,
            "error.code": code,
        }

    # Constant shorthands (avoid AttributeError for missing consts)
    class _C:
        INFO_MSG_CONNECTED = "Connected to %s:%d"

    INFO_MSG_CONNECTED = "Connected to %s:%d"
