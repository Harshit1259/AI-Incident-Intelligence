#  NeurOps Agent — Python SDK HTTP Client
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  Wraps the requests library.  Supports Basic, Bearer, API-key,
#  NTLM, and Digest authentication.  Used by REST API metric plugins
#  (cloud providers, web app monitors, network API devices, etc.).

import json as _json

import requests
import requests.auth as _auth

try:
    from requests_ntlm import HttpNtlmAuth
    _NTLM_AVAILABLE = True
except ImportError:
    _NTLM_AVAILABLE = False

from neuroopssdk.constants import Constant


class HTTPClient:

    # Auth modes
    AUTH_NONE    = "none"
    AUTH_BASIC   = "basic"
    AUTH_BEARER  = "bearer"
    AUTH_APIKEY  = "apikey"
    AUTH_NTLM    = "ntlm"
    AUTH_DIGEST  = "digest"

    DEFAULT_TIMEOUT = 30

    def __init__(self, context: dict, logger):
        self._log       = logger
        self._base_url  = context.get("base.url", "").rstrip("/")
        self._username  = context.get(Constant.USERNAME, "")
        self._password  = context.get(Constant.PASSWORD, "")
        self._token     = context.get("bearer.token", "")
        self._api_key   = context.get("api.key", "")
        self._api_key_header = context.get("api.key.header", "X-API-Key")
        self._auth_mode = context.get("auth.mode", self.AUTH_NONE).lower()
        self._timeout   = int(context.get(Constant.TIMEOUT, self.DEFAULT_TIMEOUT))
        self._verify_ssl = context.get("verify.ssl", "yes") == "yes"
        self._proxy_host = context.get("proxy.server", "")
        self._proxy_port = context.get("proxy.port", "")
        self._session   = requests.Session()
        self._setup_session()

    # ── Public API ─────────────────────────────────────────────────────────

    def get(self, path: str, params: dict = None, headers: dict = None) -> dict:
        return self._request("GET", path, params=params, extra_headers=headers)

    def post(self, path: str, body: dict = None, headers: dict = None) -> dict:
        return self._request("POST", path, body=body, extra_headers=headers)

    def put(self, path: str, body: dict = None, headers: dict = None) -> dict:
        return self._request("PUT", path, body=body, extra_headers=headers)

    def delete(self, path: str, headers: dict = None) -> dict:
        return self._request("DELETE", path, extra_headers=headers)

    def get_paginated(self, path: str, page_key: str = "page",
                      data_key: str = "data", max_pages: int = 50) -> dict:
        """
        Walk through paginated REST API responses.
        Returns all records collected from all pages.
        """
        all_records = []
        page = 1
        while page <= max_pages:
            r = self.get(path, params={page_key: page})
            if r["status"] != Constant.STATUS_SUCCEED:
                return r
            records = r["response_body"].get(data_key, [])
            if not records:
                break
            all_records.extend(records)
            page += 1
        return {
            "status":  Constant.STATUS_SUCCEED,
            "records": all_records,
            "total":   len(all_records),
        }

    def close(self):
        try:
            self._session.close()
        except Exception:
            pass

    # ── Internal ───────────────────────────────────────────────────────────

    def _setup_session(self):
        if self._auth_mode == self.AUTH_BASIC:
            self._session.auth = (_auth.HTTPBasicAuth(self._username, self._password)
                                   if self._username else None)
        elif self._auth_mode == self.AUTH_NTLM:
            if _NTLM_AVAILABLE:
                self._session.auth = HttpNtlmAuth(self._username, self._password)
            else:
                self._log.warn("NTLM auth requested but requests_ntlm not installed")
        elif self._auth_mode == self.AUTH_DIGEST:
            self._session.auth = _auth.HTTPDigestAuth(self._username, self._password)

        if self._proxy_host:
            proxy_url = f"http://{self._proxy_host}:{self._proxy_port}"
            self._session.proxies = {"http": proxy_url, "https": proxy_url}
            self._log.debugf("HTTP proxy: %s", proxy_url)

        self._session.verify = self._verify_ssl

    def _request(self, method: str, path: str, params: dict = None,
                 body: dict = None, extra_headers: dict = None) -> dict:
        url = self._base_url + "/" + path.lstrip("/") if path else self._base_url
        headers = {"Content-Type": "application/json", "Accept": "application/json"}

        if self._auth_mode == self.AUTH_BEARER:
            headers["Authorization"] = f"Bearer {self._token}"
        elif self._auth_mode == self.AUTH_APIKEY:
            headers[self._api_key_header] = self._api_key

        if extra_headers:
            headers.update(extra_headers)

        try:
            self._log.debugf("HTTP %s %s", method, url)
            resp = self._session.request(
                method=method,
                url=url,
                params=params,
                json=body,
                headers=headers,
                timeout=self._timeout,
            )
            return self._parse(resp)
        except requests.Timeout:
            return self._fail(
                f"Timeout: {method} {url} timed out after {self._timeout}s",
                Constant.ERROR_TIMEOUT,
            )
        except requests.ConnectionError:
            return self._fail(
                f"Connection error: could not reach {url}",
                Constant.ERROR_CONNECTION_FAILED,
            )
        except requests.SSLError:
            return self._fail(
                f"SSL verification failed for {url}",
                Constant.ERROR_SSL_FAILED,
            )
        except Exception as e:
            return self._fail(str(e), Constant.ERROR_INTERNAL)

    def _parse(self, resp: requests.Response) -> dict:
        status_code = resp.status_code
        body = None
        try:
            body = resp.json()
        except Exception:
            body = resp.text

        if status_code == 401:
            return self._fail(
                f"Authentication failed (401) for {resp.url}",
                Constant.ERROR_INVALID_CREDENTIALS,
            )
        if status_code == 403:
            return self._fail(
                f"Access forbidden (403) for {resp.url}",
                Constant.ERROR_UNAUTHORIZED_ACCESS,
            )
        if status_code >= 400:
            return self._fail(
                f"HTTP {status_code} from {resp.url}: {str(body)[:200]}",
                Constant.ERROR_BAD_RESPONSE,
            )
        return {
            "status":        Constant.STATUS_SUCCEED,
            "status_code":   status_code,
            "response_body": body,
        }

    def _fail(self, message: str, code: str = Constant.ERROR_INTERNAL) -> dict:
        self._log.error(message)
        return {
            "status":     Constant.STATUS_FAIL,
            "error":      message,
            "error.code": code,
        }
