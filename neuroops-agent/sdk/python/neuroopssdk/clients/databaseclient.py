#  NeurOps Agent — Python SDK Database Client
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  Unified database client supporting MySQL, PostgreSQL, MSSQL,
#  Oracle, SAP HANA, IBM DB2, and MongoDB.
#  Used by database metric plugins.
#
#  Driver mapping (all included in python-embedded):
#    MySQL       → mysql.connector
#    PostgreSQL  → psycopg2  (via pyodbc fallback)
#    MSSQL       → pyodbc
#    Oracle      → cx_Oracle
#    HANA        → pyodbc (hdbcli driver)
#    DB2         → ibm_db_dbi
#    MongoDB     → pymongo

from neuroopssdk.constants import Constant


class DatabaseClient:

    # Supported database types
    MYSQL      = "mysql"
    POSTGRESQL = "postgresql"
    MSSQL      = "sqlserver"
    ORACLE     = "oracle"
    HANA       = "hana"
    DB2        = "db2"
    MONGODB    = "mongodb"

    DEFAULT_TIMEOUT = 30

    def __init__(self, context: dict, logger):
        self._log      = logger
        self._host     = context.get(Constant.OBJECT_IP, "")
        self._port     = context.get(Constant.PORT)
        self._username = context.get(Constant.USERNAME, "")
        self._password = context.get(Constant.PASSWORD, "")
        self._database = context.get("database", "")
        self._db_type  = context.get("database.type", self.MYSQL).lower()
        self._timeout  = int(context.get(Constant.TIMEOUT, self.DEFAULT_TIMEOUT))
        self._ssl      = context.get("ssl.enabled", "no") == "yes"
        self._conn     = None

    # ── Lifecycle ──────────────────────────────────────────────────────────

    def connect(self) -> dict:
        """Open a database connection."""
        try:
            self._conn = self._make_connection()
            self._log.infof("Connected to %s database at %s", self._db_type, self._host)
            return {"status": Constant.STATUS_SUCCEED}
        except Exception as e:
            return self._fail(str(e), Constant.ERROR_CONNECTION_FAILED)

    def disconnect(self):
        if self._conn:
            try:
                self._conn.close()
            except Exception:
                pass
            self._conn = None

    # ── Query ──────────────────────────────────────────────────────────────

    def query(self, sql: str, params=None) -> dict:
        """
        Execute a SQL SELECT query and return rows as list of dicts.
        Returns: {"status": "succeed", "rows": [...], "row_count": N}
        """
        if not self._conn:
            return self._fail("Not connected", Constant.ERROR_CONNECTION_FAILED)
        try:
            cur = self._conn.cursor()
            if params:
                cur.execute(sql, params)
            else:
                cur.execute(sql)
            columns = [d[0] for d in (cur.description or [])]
            rows = [dict(zip(columns, row)) for row in (cur.fetchall() or [])]
            cur.close()
            return {
                "status":    Constant.STATUS_SUCCEED,
                "rows":      rows,
                "row_count": len(rows),
            }
        except Exception as e:
            return self._fail(f"Query failed: {e}", Constant.ERROR_INTERNAL)

    def execute(self, sql: str, params=None) -> dict:
        """Execute a non-SELECT statement (INSERT, UPDATE, DELETE, DDL)."""
        if not self._conn:
            return self._fail("Not connected", Constant.ERROR_CONNECTION_FAILED)
        try:
            cur = self._conn.cursor()
            if params:
                cur.execute(sql, params)
            else:
                cur.execute(sql)
            rows_affected = cur.rowcount
            self._conn.commit()
            cur.close()
            return {
                "status":       Constant.STATUS_SUCCEED,
                "rows_affected": rows_affected,
            }
        except Exception as e:
            return self._fail(f"Execute failed: {e}", Constant.ERROR_INTERNAL)

    def query_single(self, sql: str, params=None):
        """Return the first cell of the first row, or None."""
        r = self.query(sql, params)
        if r["status"] != Constant.STATUS_SUCCEED or not r["rows"]:
            return None
        row = r["rows"][0]
        return next(iter(row.values()), None)

    # ── Internal ───────────────────────────────────────────────────────────

    def _make_connection(self):
        port  = int(self._port) if self._port else self._default_port()
        dtype = self._db_type

        if dtype == self.MYSQL:
            import mysql.connector
            return mysql.connector.connect(
                host=self._host, port=port,
                user=self._username, password=self._password,
                database=self._database or None,
                connection_timeout=self._timeout,
                ssl_disabled=not self._ssl,
            )

        elif dtype == self.POSTGRESQL:
            import psycopg2
            return psycopg2.connect(
                host=self._host, port=port,
                user=self._username, password=self._password,
                dbname=self._database or "postgres",
                connect_timeout=self._timeout,
            )

        elif dtype == self.MSSQL:
            import pyodbc
            dsn = (
                f"DRIVER={{ODBC Driver 18 for SQL Server}};"
                f"SERVER={self._host},{port};"
                f"DATABASE={self._database or 'master'};"
                f"UID={self._username};PWD={self._password};"
                f"TrustServerCertificate={'yes' if not self._ssl else 'no'};"
                f"Connection Timeout={self._timeout};"
            )
            return pyodbc.connect(dsn)

        elif dtype == self.ORACLE:
            import cx_Oracle
            dsn = cx_Oracle.makedsn(self._host, port, service_name=self._database)
            return cx_Oracle.connect(self._username, self._password, dsn,
                                     encoding="UTF-8")

        elif dtype == self.HANA:
            try:
                from hdbcli import dbapi as hdb
                return hdb.connect(
                    address=self._host, port=port,
                    user=self._username, password=self._password,
                )
            except ImportError:
                import pyodbc
                dsn = (
                    f"DRIVER={{HDBODBC}};"
                    f"SERVERNODE={self._host}:{port};"
                    f"UID={self._username};PWD={self._password};"
                )
                return pyodbc.connect(dsn)

        elif dtype == self.DB2:
            import ibm_db_dbi
            conn_str = (
                f"DATABASE={self._database};HOSTNAME={self._host};"
                f"PORT={port};PROTOCOL=TCPIP;"
                f"UID={self._username};PWD={self._password};"
            )
            return ibm_db_dbi.connect(conn_str, "", "")

        elif dtype == self.MONGODB:
            from pymongo import MongoClient
            uri = f"mongodb://{self._username}:{self._password}@{self._host}:{port}/"
            if not self._username:
                uri = f"mongodb://{self._host}:{port}/"
            client = MongoClient(uri, serverSelectionTimeoutMS=self._timeout * 1000)
            client.server_info()    # force connection check
            return client

        else:
            raise ValueError(f"Unsupported database type: {self._db_type}")

    def _default_port(self) -> int:
        return {
            self.MYSQL: 3306, self.POSTGRESQL: 5432, self.MSSQL: 1433,
            self.ORACLE: 1521, self.HANA: 30015, self.DB2: 50000,
            self.MONGODB: 27017,
        }.get(self._db_type, 3306)

    def _fail(self, message: str, code: str = Constant.ERROR_INTERNAL) -> dict:
        self._log.errorf("DatabaseClient [%s/%s]: %s", self._db_type, self._host, message)
        return {
            "status":     Constant.STATUS_FAIL,
            "error":      message,
            "error.code": code,
        }
