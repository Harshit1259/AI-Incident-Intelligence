package services

// Package services — Failure Signature Library
//
// A curated catalog of 200+ failure patterns organized by technology.
// Each Signature captures:
//   - Keywords that trigger the match (OR semantics within Keywords)
//   - A human-readable cause and remediation hint
//   - Severity and category for LLM context enrichment
//
// Usage: MatchSignatures(event/incident text) → []Signature
// The context-assembly pipeline feeds matches into the LLM prompt so the
// model reasons with domain-specific knowledge rather than general priors.

import "strings"

// Signature describes a known failure pattern.
type Signature struct {
	ID          string   // Unique slug
	Category    string   // Technology category
	Keywords    []string // Any keyword match triggers this signature
	Cause       string   // Concise human-readable cause
	Remediation string   // First recommended remediation step
	Severity    string   // Expected severity: critical | high | medium | low
}

// ─────────────────────────────────────────────────────
// Global library (exported for testing / LLM enrichment)
// ─────────────────────────────────────────────────────

// Library is the full pre-built failure signature catalog.
var Library = []Signature{

	// ══════════════════════════════════════════════════
	// REDIS (30 patterns)
	// ══════════════════════════════════════════════════
	{"redis-oom", "redis", []string{"redis out of memory", "redis oom", "redis maxmemory", "err: oom command"}, "Redis has reached its maxmemory limit", "Evict keys with `redis-cli flushdb` (non-prod) or increase maxmemory; check eviction policy", "critical"},
	{"redis-conn-refused", "redis", []string{"connection refused 6379", "redis connection refused", "dial tcp.*6379.*refused"}, "Redis is not accepting connections", "Verify Redis process is running: `systemctl status redis`", "critical"},
	{"redis-timeout", "redis", []string{"redis timeout", "redis read timeout", "redis write timeout", "context deadline exceeded redis"}, "Redis response latency exceeds client timeout", "Check Redis slowlog: `SLOWLOG GET 25`; inspect keyspace size", "high"},
	{"redis-cluster-down", "redis", []string{"cluster is down", "cluster_state:fail", "redis cluster failed"}, "Redis cluster has entered a failed state", "Run `redis-cli cluster info` and `cluster nodes` to identify the failed shard", "critical"},
	{"redis-replica-lag", "redis", []string{"replication lag", "replica lag", "master_repl_offset", "slave_repl_offset"}, "Redis replica is lagging behind the primary", "Check `INFO replication`; inspect network bandwidth between primary and replica", "high"},
	{"redis-aof-error", "redis", []string{"aof rewrite", "aof background save error", "aof failed"}, "Redis AOF persistence failed", "Check disk space and permissions on the AOF directory; disable AOF temporarily if critical", "high"},
	{"redis-rdb-save-error", "redis", []string{"rdb save failed", "background save error", "bgsave error"}, "Redis RDB snapshot failed", "Ensure disk has sufficient space; check `LASTSAVE` and `INFO persistence`", "medium"},
	{"redis-client-limit", "redis", []string{"max number of clients reached", "redis clients limit", "err max clients"}, "Redis has reached its `maxclients` limit", "Increase `maxclients` in redis.conf or reduce connection pool size in the application", "high"},
	{"redis-eviction", "redis", []string{"keys evicted", "evicted_keys", "redis eviction"}, "Redis is actively evicting keys due to memory pressure", "Increase Redis memory or audit large keys with `redis-cli --bigkeys`", "medium"},
	{"redis-blocked-clients", "redis", []string{"blocked_clients", "client is blocked", "brpop timeout"}, "Clients are blocked waiting on Redis list/stream operations", "Investigate long-running BLPOP/BRPOP commands; consider timeout tuning", "medium"},
	{"redis-high-cpu", "redis", []string{"redis cpu", "redis high cpu", "redis 100% cpu"}, "Redis process consuming excessive CPU", "Run `redis-cli debug sleep 0` to interrupt; profile with `redis-cli --latency`", "high"},
	{"redis-keyspace-miss", "redis", []string{"keyspace_misses", "cache miss rate", "high cache miss"}, "High cache miss rate detected in Redis", "Review TTL settings and cache warming strategy", "medium"},
	{"redis-sentinel-failover", "redis", []string{"sentinel failover", "sentinel promoted", "+failover-triggered"}, "Redis Sentinel triggered a failover", "Verify new primary is healthy; update application connection strings if needed", "high"},
	{"redis-network-error", "redis", []string{"redis network", "redis broken pipe", "redis i/o timeout"}, "Network error communicating with Redis", "Check network path between app and Redis; verify Redis bind address", "high"},
	{"redis-script-error", "redis", []string{"lua script error", "eval error", "redis script"}, "Redis Lua script execution failed", "Review Lua script logic; check for prohibited commands inside EVAL", "medium"},
	{"redis-config-rewrite", "redis", []string{"config rewrite failed", "redis config rewrite"}, "Redis failed to persist configuration changes", "Check file permissions on redis.conf", "low"},
	{"redis-slow-query", "redis", []string{"redis slow", "slowlog", "redis latency spike"}, "Redis slow query detected", "Review SLOWLOG; consider adding indexes or restructuring data model", "medium"},
	{"redis-pub-sub-drop", "redis", []string{"message dropped", "pubsub overflow", "redis subscribe overflow"}, "Redis pub/sub messages are being dropped", "Increase client output buffer limits or reduce message rate", "medium"},
	{"redis-master-down", "redis", []string{"master is down", "cannot reach master", "redis primary unreachable"}, "Redis primary node is unreachable", "Trigger manual failover or investigate network partition", "critical"},
	{"redis-auth-failure", "redis", []string{"redis auth failed", "noauth authentication", "redis wrong password"}, "Redis authentication failed", "Verify REDIS_PASSWORD environment variable matches requirepass in redis.conf", "high"},
	{"redis-tls-error", "redis", []string{"redis tls error", "ssl handshake failed redis", "redis certificate"}, "Redis TLS/SSL connection error", "Verify certificates are valid and not expired; check tls-cert-file in config", "high"},
	{"redis-version-mismatch", "redis", []string{"redis version", "unsupported redis version", "redis command not supported"}, "Redis command unsupported in this version", "Check client library compatibility with the Redis server version", "medium"},
	{"redis-dump-corrupt", "redis", []string{"dump is corrupted", "rdb checksum error", "redis dump"}, "Redis RDB dump file is corrupted", "Restore from a previous backup; do not restart Redis with the corrupted RDB", "critical"},
	{"redis-fork-fail", "redis", []string{"can't save in background: fork error", "fork failed", "redis fork"}, "Redis fork() for background save failed", "Check system vm.overcommit_memory setting; free system memory", "high"},
	{"redis-large-keys", "redis", []string{"big key", "large key", "redis key size"}, "Large keys detected causing latency", "Use OBJECT ENCODING and MEMORY USAGE to audit; break large keys into smaller ones", "medium"},
	{"redis-flushall", "redis", []string{"flushall", "flushdb"}, "Redis keyspace was flushed", "Audit who triggered the flush; check access control lists", "high"},
	{"redis-latency-spike", "redis", []string{"intrinsic latency", "redis latency", "redis response time"}, "Redis latency spike detected", "Run `redis-cli --latency-history`; check for AOF/RDB operations running in background", "high"},
	{"redis-socket-error", "redis", []string{"redis socket error", "redis unix socket", "redis accept error"}, "Redis socket error", "Check socket file permissions and listen backlog settings", "medium"},
	{"redis-cluster-resharding", "redis", []string{"cluster resharding", "slot migration", "migrating slot"}, "Redis cluster slot migration in progress causing latency", "Monitor migration progress with `cluster info`; avoid writes to migrating slots", "medium"},
	{"redis-watchdog", "redis", []string{"redis watchdog", "server soft limit", "server hard limit"}, "Redis server watchdog triggered", "Review Redis system resource limits; check logs for slow operations", "high"},

	// ══════════════════════════════════════════════════
	// POSTGRESQL (35 patterns)
	// ══════════════════════════════════════════════════
	{"pg-conn-refused", "postgresql", []string{"connection refused 5432", "postgres connection refused", "dial tcp.*5432.*refused"}, "PostgreSQL is not accepting connections", "Check pg_hba.conf and PostgreSQL process: `systemctl status postgresql`", "critical"},
	{"pg-max-conn", "postgresql", []string{"too many connections", "remaining connection slots", "max_connections"}, "PostgreSQL max_connections limit reached", "Increase max_connections or deploy PgBouncer connection pooler", "critical"},
	{"pg-deadlock", "postgresql", []string{"deadlock detected", "deadlock found", "pg deadlock"}, "Database deadlock detected", "Review transaction order in application code; add retry logic for deadlock errors", "high"},
	{"pg-lock-wait", "postgresql", []string{"lock wait timeout", "waiting for lock", "lock timeout", "relation.*lock"}, "Query waiting on table/row lock", "Identify blocking query with `pg_stat_activity`; consider lock_timeout setting", "high"},
	{"pg-slow-query", "postgresql", []string{"slow query", "query time exceeded", "pg slow", "statement timeout"}, "Slow query detected in PostgreSQL", "Enable `pg_stat_statements`; run EXPLAIN ANALYZE on the slow query", "medium"},
	{"pg-replication-lag", "postgresql", []string{"replication lag", "wal receiver", "pg_stat_replication", "replication slot lag"}, "PostgreSQL replication is lagging", "Check `pg_stat_replication`; verify network between primary and replica", "high"},
	{"pg-disk-full", "postgresql", []string{"no space left on device", "disk full postgres", "could not write to file"}, "PostgreSQL data directory disk is full", "Free disk space immediately; archive or drop unused tables/indexes", "critical"},
	{"pg-autovacuum", "postgresql", []string{"autovacuum", "table bloat", "dead tuples"}, "Table bloat or autovacuum issue detected", "Run `VACUUM ANALYZE` manually; tune autovacuum_vacuum_scale_factor", "medium"},
	{"pg-checkpoint-warning", "postgresql", []string{"checkpoint occurring too frequently", "checkpoint warning", "wal checkpoint"}, "Frequent WAL checkpoints indicate high write load", "Increase checkpoint_completion_target and max_wal_size", "medium"},
	{"pg-out-of-memory", "postgresql", []string{"out of memory postgres", "shared_buffers", "work_mem exceeded"}, "PostgreSQL ran out of memory", "Tune work_mem and shared_buffers; check for memory-intensive queries", "critical"},
	{"pg-crash", "postgresql", []string{"server process exited with exit code", "postgres crashed", "postmaster exited"}, "PostgreSQL server process crashed", "Check PostgreSQL logs for crash context; run `pg_resetwal` only if necessary", "critical"},
	{"pg-ssl-error", "postgresql", []string{"ssl error postgres", "ssl connection required", "certificate verify failed postgres"}, "PostgreSQL SSL handshake failure", "Verify SSL certificates and pg_hba.conf ssl settings", "high"},
	{"pg-index-corrupt", "postgresql", []string{"invalid page in block", "index.*corrupt", "relation.*invalid"}, "PostgreSQL index corruption detected", "Run `REINDEX TABLE` on affected table; restore from backup if data is corrupted", "critical"},
	{"pg-sequence-overflow", "postgresql", []string{"sequence overflow", "nextval.*out of range", "integer out of range"}, "Integer primary key sequence overflow", "Alter sequence or change column type to BIGSERIAL", "critical"},
	{"pg-peer-auth", "postgresql", []string{"peer authentication failed", "ident authentication failed"}, "PostgreSQL peer/ident authentication failed", "Update pg_hba.conf to use md5/scram-sha-256 authentication for the connecting user", "high"},
	{"pg-connection-reset", "postgresql", []string{"connection reset by peer", "connection terminated", "server closed connection"}, "PostgreSQL connection unexpectedly terminated", "Check PostgreSQL idle_in_transaction_session_timeout and server restart events", "high"},
	{"pg-archive-error", "postgresql", []string{"archive command failed", "wal archive", "archive_status"}, "WAL archiving failed", "Check archive_command output; verify archive destination is accessible", "high"},
	{"pg-long-transaction", "postgresql", []string{"long running transaction", "idle in transaction", "transaction age"}, "Long-running or idle transaction detected", "Kill idle transactions with `pg_terminate_backend()`; set idle_in_transaction_session_timeout", "medium"},
	{"pg-constraint-violation", "postgresql", []string{"unique violation", "foreign key violation", "check constraint", "not null violation"}, "Database constraint violation", "Review application logic; ensure data integrity before writes", "medium"},
	{"pg-extension-missing", "postgresql", []string{"extension.*not found", "could not open extension", "pg_extension"}, "Required PostgreSQL extension missing", "Install missing extension: `CREATE EXTENSION IF NOT EXISTS <name>`", "high"},
	{"pg-standby-conflict", "postgresql", []string{"conflict with recovery", "hot standby conflict", "canceling statement due to conflict"}, "Query cancelled due to standby conflict", "Increase max_standby_streaming_delay; consider hot_standby_feedback=on", "medium"},
	{"pg-statistics-stale", "postgresql", []string{"stale statistics", "planner used wrong plan", "bad query plan"}, "Stale table statistics causing bad query plans", "Run `ANALYZE` on affected tables; check autovacuum schedule", "medium"},
	{"pg-tablespace-full", "postgresql", []string{"tablespace.*full", "could not extend file"}, "PostgreSQL tablespace disk full", "Add disk space or move tablespace to a larger volume", "critical"},
	{"pg-permission-denied", "postgresql", []string{"permission denied for table", "permission denied for schema", "must be owner"}, "PostgreSQL permission denied", "Grant required permissions: `GRANT SELECT ON ... TO ...`", "medium"},
	{"pg-encoding-error", "postgresql", []string{"invalid byte sequence", "encoding error", "multibyte character"}, "Character encoding error in PostgreSQL", "Verify database encoding and client encoding match; sanitize input data", "medium"},
	{"pg-walsender-error", "postgresql", []string{"walsender", "replication connection failed", "wal sender"}, "WAL sender process error", "Check replication slot status with `pg_replication_slots`", "high"},
	{"pg-prepared-statement", "postgresql", []string{"prepared statement.*does not exist", "invalid prepared statement"}, "Invalid prepared statement reference", "Ensure application reconnects and re-prepares statements after connection loss", "medium"},
	{"pg-json-error", "postgresql", []string{"invalid input syntax for type json", "jsonb error", "json parse error"}, "Invalid JSON value inserted into JSONB column", "Validate JSON on the application side before inserting", "medium"},
	{"pg-toast-error", "postgresql", []string{"toast table", "value too long for type", "out-of-line value"}, "PostgreSQL TOAST storage error", "Check for excessively large field values; consider TEXT compression", "medium"},
	{"pg-crash-recovery", "postgresql", []string{"database system was shut down", "entering standby mode", "consistent recovery state"}, "PostgreSQL crash recovery in progress", "Wait for recovery to complete; monitor with `pg_is_in_recovery()`", "high"},
	{"pg-parallel-worker", "postgresql", []string{"parallel worker", "could not resize shared memory", "parallel query failed"}, "Parallel query worker failure", "Reduce max_parallel_workers_per_gather or increase system shared memory", "medium"},
	{"pg-clog-error", "postgresql", []string{"clog error", "transaction status", "pg_xact"}, "Transaction commit log error", "Critical: stop writes and contact DBA; may require point-in-time recovery", "critical"},
	{"pg-fdw-error", "postgresql", []string{"foreign-data wrapper", "fdw error", "postgres_fdw"}, "Foreign Data Wrapper error", "Verify remote server connection and credentials in foreign server definition", "high"},
	{"pg-logical-replication", "postgresql", []string{"logical replication", "publication", "subscription", "replication origin"}, "Logical replication error", "Check `pg_stat_subscription`; verify publication exists on primary", "high"},
	{"pg-vacuum-freeze", "postgresql", []string{"transaction id wraparound", "vacuum to prevent wraparound", "xid wraparound"}, "Transaction ID wraparound risk", "Run `VACUUM FREEZE` immediately; this is critical to prevent data corruption", "critical"},

	// ══════════════════════════════════════════════════
	// NODE.JS (35 patterns)
	// ══════════════════════════════════════════════════
	{"node-oom", "nodejs", []string{"javascript heap out of memory", "heap out of memory", "fatal error: oom"}, "Node.js process ran out of heap memory", "Increase `--max-old-space-size`; profile heap with Chrome DevTools or `clinic heap`", "critical"},
	{"node-unhandled-rejection", "nodejs", []string{"unhandledrejection", "unhandled promise rejection", "uncaughtexception"}, "Unhandled promise rejection or exception", "Add `.catch()` handlers or `process.on('unhandledRejection')` global handler", "high"},
	{"node-event-loop-lag", "nodejs", []string{"event loop lag", "event loop blocked", "event loop delay"}, "Node.js event loop is blocked", "Profile with `clinic doctor`; identify CPU-intensive synchronous code", "high"},
	{"node-crash", "nodejs", []string{"node process crashed", "pm2 restarted", "process exited code 1", "signal sigsegv"}, "Node.js process crashed and restarted", "Check application logs for the exception before crash; review PM2/systemd logs", "critical"},
	{"node-timeout", "nodejs", []string{"request timeout", "socket hang up", "ETIMEDOUT", "ECONNRESET"}, "Network timeout in Node.js service", "Check downstream service health; increase timeout if expected; add retry logic", "high"},
	{"node-econnrefused", "nodejs", []string{"ECONNREFUSED", "connection refused node", "connect ECONNREFUSED"}, "Node.js cannot connect to a dependency", "Verify the target service is running and the address/port is correct", "high"},
	{"node-memory-leak", "nodejs", []string{"memory leak", "heap growing", "rss growing", "node memory"}, "Suspected Node.js memory leak", "Take heap snapshot before/after with `process.memoryUsage()`; use `heapdump`", "high"},
	{"node-libuv-thread", "nodejs", []string{"libuv thread", "thread pool", "uv_threadpool_size"}, "libuv thread pool exhausted", "Increase UV_THREADPOOL_SIZE env var; audit blocking I/O in async operations", "medium"},
	{"node-dns-fail", "nodejs", []string{"ENOTFOUND", "getaddrinfo ENOTFOUND", "dns lookup failed"}, "DNS resolution failure in Node.js", "Check DNS server configuration; verify hostname is correct", "high"},
	{"node-cert-error", "nodejs", []string{"UNABLE_TO_VERIFY_LEAF_SIGNATURE", "certificate expired", "SELF_SIGNED_CERT"}, "TLS certificate error in Node.js", "Renew certificate; verify CA chain; set NODE_EXTRA_CA_CERTS if using custom CA", "high"},
	{"node-import-error", "nodejs", []string{"cannot find module", "module not found", "es module"}, "Module not found error", "Run `npm install`; check node_modules and package.json", "high"},
	{"node-syntax-error", "nodejs", []string{"syntaxerror", "unexpected token", "unexpected end of json"}, "Syntax error in Node.js code", "Fix the syntax error in the indicated file and line", "critical"},
	{"node-file-descriptor", "nodejs", []string{"emfile", "too many open files", "enfile"}, "Node.js process exceeded file descriptor limit", "Increase system `ulimit -n`; audit and close unused file handles", "high"},
	{"node-worker-thread", "nodejs", []string{"worker thread", "worker_threads", "workerthread error"}, "Worker thread error in Node.js", "Check worker thread message handling and error propagation", "medium"},
	{"node-cluster-disconnect", "nodejs", []string{"cluster worker disconnected", "cluster worker died", "ipc channel closed"}, "Node.js cluster worker disconnected", "Monitor worker respawn rate; investigate worker exit reason in logs", "high"},
	{"node-express-crash", "nodejs", []string{"express", "app.listen error", "express error handler"}, "Express.js application error", "Add error-handling middleware: `app.use((err, req, res, next) => {...})`", "high"},
	{"node-db-pool", "nodejs", []string{"pool is draining", "pool exhausted", "connection pool", "knex pool"}, "Database connection pool exhausted in Node.js", "Increase pool max size; ensure connections are released after queries", "high"},
	{"node-buffer-overflow", "nodejs", []string{"buffer overflow", "buffer too large", "max buffer exceeded"}, "Buffer size limit exceeded", "Stream large responses instead of buffering; increase maxBuffer option", "medium"},
	{"node-grpc-error", "nodejs", []string{"grpc error", "grpc unavailable", "grpc deadline exceeded"}, "gRPC call failure in Node.js", "Check gRPC server health; verify proto definitions match", "high"},
	{"node-env-missing", "nodejs", []string{"undefined environment variable", "process.env", "missing env"}, "Required environment variable not set", "Set the missing environment variable in your deployment configuration", "high"},
	{"node-esm-error", "nodejs", []string{"err_require_esm", "cannot use import statement", "esm module error"}, "ES Module compatibility error", "Convert to ES Modules or use dynamic `import()` for ESM packages", "medium"},
	{"node-jest-timeout", "nodejs", []string{"jest timeout", "test timed out", "async test"}, "Jest test timeout (not production but signals integration issue)", "Increase jest timeout or fix async operations in tests", "low"},
	{"node-fastify-error", "nodejs", []string{"fastify error", "fastify validation", "fastify 500"}, "Fastify framework error", "Check Fastify validation schemas and error handlers", "high"},
	{"node-websocket-error", "nodejs", []string{"websocket error", "ws error", "websocket connection closed"}, "WebSocket connection error", "Verify WebSocket server health; implement reconnection logic on client", "medium"},
	{"node-mongoose-error", "nodejs", []string{"mongoose error", "mongoose validation", "mongodb connection error"}, "Mongoose/MongoDB error in Node.js", "Check MongoDB connection string and server health; validate schema", "high"},
	{"node-sequelize-error", "nodejs", []string{"sequelize error", "sequelize validation", "sequelize connection"}, "Sequelize ORM error", "Check database connection; run pending migrations", "high"},
	{"node-axios-error", "nodejs", []string{"axios error", "axioserror", "request failed with status"}, "HTTP client (Axios) request failure", "Check upstream service health; handle HTTP error codes in catch blocks", "medium"},
	{"node-jest-crash", "nodejs", []string{"jest crashed", "worker process failed", "jest force exit"}, "Jest test runner crashed", "Run tests with `--detectOpenHandles` to find resource leaks", "medium"},
	{"node-sharp-error", "nodejs", []string{"sharp error", "libvips error", "image processing failed"}, "Image processing failure (sharp/libvips)", "Verify libvips version compatibility; check image format is supported", "medium"},
	{"node-rate-limit", "nodejs", []string{"rate limit exceeded", "429 too many requests", "throttled"}, "Rate limit exceeded", "Implement exponential backoff; review rate limit configuration", "medium"},
	{"node-static-file", "nodejs", []string{"static file error", "serve-static", "cannot GET /"}, "Static file serving error", "Verify static file path and Express `express.static()` configuration", "low"},
	{"node-multer-error", "nodejs", []string{"multer error", "file upload error", "unexpected field"}, "File upload error (Multer)", "Check field name in multipart form matches Multer configuration", "medium"},
	{"node-session-error", "nodejs", []string{"session error", "cookie error", "express-session"}, "Session management error in Node.js", "Verify session store configuration and cookie secret", "medium"},
	{"node-cron-error", "nodejs", []string{"cron error", "scheduled job failed", "cron job"}, "Scheduled cron job failure", "Check cron expression syntax; review job error handler", "medium"},
	{"node-child-process", "nodejs", []string{"child process error", "spawn error", "execfile error"}, "Child process error in Node.js", "Verify the spawned command exists and has execute permissions", "high"},

	// ══════════════════════════════════════════════════
	// PYTHON (35 patterns)
	// ══════════════════════════════════════════════════
	{"py-oom", "python", []string{"memoreerror", "python out of memory", "killed signal 9"}, "Python process ran out of memory", "Profile with `tracemalloc`; use generators instead of lists for large datasets", "critical"},
	{"py-timeout", "python", []string{"readtimeouterror", "connecttimeout", "requests.exceptions.timeout"}, "HTTP request timeout in Python service", "Increase timeout parameter; add retry with `urllib3.util.retry`", "high"},
	{"py-crash", "python", []string{"segmentation fault python", "python crashed", "core dumped python"}, "Python process segfault (usually C extension)", "Check for incompatible native extensions; use `faulthandler` for debug info", "critical"},
	{"py-import-error", "python", []string{"importerror", "modulenotfounderror", "no module named"}, "Python module import error", "Run `pip install -r requirements.txt`; verify virtualenv is activated", "high"},
	{"py-db-conn", "python", []string{"sqlalchemy connection", "psycopg2 connection", "database connection error python"}, "Database connection failure in Python", "Check DATABASE_URL environment variable; verify DB is reachable", "high"},
	{"py-celery-worker", "python", []string{"celery worker", "celery task failed", "kombu error"}, "Celery worker failure", "Check broker connectivity; run `celery inspect active` to diagnose", "high"},
	{"py-django-error", "python", []string{"django error", "django.core.exceptions", "500 internal server error django"}, "Django application error", "Check `DJANGO_DEBUG` logs; run `manage.py check` for configuration issues", "high"},
	{"py-flask-error", "python", []string{"flask error", "werkzeug error", "500 flask"}, "Flask application error", "Enable Flask debug mode locally; check application error logs", "high"},
	{"py-fastapi-error", "python", []string{"fastapi error", "starlette error", "uvicorn error"}, "FastAPI/Uvicorn error", "Check FastAPI validation errors and exception handlers", "high"},
	{"py-keyerror", "python", []string{"keyerror", "key error python"}, "Dictionary key not found", "Add `.get()` with default value; validate dict structure before access", "medium"},
	{"py-typeerror", "python", []string{"typeerror", "type error python", "object is not subscriptable"}, "Type error in Python", "Add type checking; use `isinstance()` guards before operations", "medium"},
	{"py-valueerror", "python", []string{"valueerror", "invalid literal", "value error python"}, "Invalid value in Python code", "Validate input data before processing; add try/except for parsing", "medium"},
	{"py-recursion", "python", []string{"recursionerror", "maximum recursion depth", "stack overflow python"}, "Python recursion limit exceeded", "Increase `sys.setrecursionlimit()` or convert to iterative approach", "high"},
	{"py-gc-pressure", "python", []string{"gc overhead", "garbage collection", "python gc"}, "Python garbage collection pressure", "Profile with `gc` module; reduce object creation in hot paths", "medium"},
	{"py-pickle-error", "python", []string{"pickling error", "pickle protocol", "unpickling error"}, "Python pickle serialization error", "Verify pickle compatibility between Python versions; use JSON for cross-version data", "medium"},
	{"py-subprocess-error", "python", []string{"subprocess failed", "subprocess.calledprocesserror", "returncode"}, "Subprocess execution failure", "Check command exists and has correct permissions; capture stderr for details", "medium"},
	{"py-redis-py", "python", []string{"redis.exceptions", "redis connection error python", "redis responseError"}, "Redis client error in Python", "Check Redis connectivity; verify redis-py version compatibility", "high"},
	{"py-boto3-error", "python", []string{"boto3 error", "botocore error", "aws error python", "clienterror"}, "AWS SDK (boto3) error", "Check AWS credentials; verify IAM permissions for the operation", "high"},
	{"py-pydantic-error", "python", []string{"pydantic error", "validation error pydantic", "validationerror"}, "Pydantic validation error", "Check input data schema against Pydantic model definition", "medium"},
	{"py-alembic-error", "python", []string{"alembic error", "migration failed", "alembic revision"}, "Alembic database migration failure", "Run `alembic current` to check migration state; fix migration script", "high"},
	{"py-gunicorn-timeout", "python", []string{"gunicorn timeout", "worker timeout", "gunicorn worker"}, "Gunicorn worker timeout", "Increase `--timeout`; optimize slow request handlers", "high"},
	{"py-ssl-error", "python", []string{"ssl error python", "certificate verify failed", "ssl: certificate_verify_failed"}, "SSL certificate verification failure in Python", "Update certifi: `pip install --upgrade certifi`; verify certificate chain", "high"},
	{"py-thread-error", "python", []string{"thread error python", "threading.thread", "daemonic thread"}, "Threading error in Python", "Check thread synchronization; use locks to prevent race conditions", "high"},
	{"py-async-error", "python", []string{"asyncioerror", "coroutine was never awaited", "event loop closed"}, "Python asyncio error", "Add `await` before coroutine calls; ensure event loop is running", "high"},
	{"py-json-error", "python", []string{"json.decodeerror", "invalid json python", "json loads error"}, "JSON parsing error in Python", "Validate JSON format; handle JSONDecodeError exception", "medium"},
	{"py-pandas-error", "python", []string{"pandas error", "dataframe error", "keyerror pandas"}, "Pandas DataFrame error", "Check column names and data types; use `.get()` for safe column access", "medium"},
	{"py-numpy-error", "python", []string{"numpy error", "array shape mismatch", "numpy broadcast"}, "NumPy array operation error", "Verify array dimensions match operation requirements", "medium"},
	{"py-zerodivision", "python", []string{"zerodivisionerror", "division by zero python"}, "Division by zero error", "Add zero-check guard before division operations", "medium"},
	{"py-file-not-found", "python", []string{"filenotfounderror", "no such file or directory python"}, "File not found in Python", "Verify file path is correct; use `pathlib.Path.exists()` check", "medium"},
	{"py-permission-error", "python", []string{"permissionerror python", "permission denied python", "access denied python"}, "File/OS permission error in Python", "Check file/directory permissions; run process with appropriate user", "medium"},
	{"py-encoding-error", "python", []string{"unicodedecodeError", "encoding error python", "codec can't decode"}, "Character encoding error in Python", "Specify encoding explicitly: `open(file, encoding='utf-8')`; handle surrogate errors", "medium"},
	{"py-signal-error", "python", []string{"signal handler", "sigterm python", "signal received"}, "Python process received termination signal", "Implement graceful shutdown handler for SIGTERM", "medium"},
	{"py-openai-error", "python", []string{"openai error", "openai.error", "openai ratelimit"}, "OpenAI API error in Python", "Check API key and rate limits; implement exponential backoff", "high"},
	{"py-kafka-error", "python", []string{"kafka error python", "confluent_kafka", "producer error"}, "Kafka producer/consumer error in Python", "Check Kafka broker connectivity; verify topic exists and ACLs", "high"},
	{"py-uvicorn-crash", "python", []string{"uvicorn crashed", "uvicorn shutdown", "uvicorn fatal"}, "Uvicorn ASGI server crashed", "Check application startup error; verify all imports succeed", "critical"},

	// ══════════════════════════════════════════════════
	// KUBERNETES (40 patterns)
	// ══════════════════════════════════════════════════
	{"k8s-crashloop", "kubernetes", []string{"crashloopbackoff", "back-off restarting failed container", "crash loop"}, "Pod is in CrashLoopBackOff", "Run `kubectl logs <pod> --previous` to see crash reason; fix application error", "critical"},
	{"k8s-oom-kill", "kubernetes", []string{"oomkilled", "oom kill", "out of memory killed"}, "Pod killed by OOM (Out Of Memory) killer", "Increase pod memory limit; optimize application memory usage", "critical"},
	{"k8s-pending", "kubernetes", []string{"pod pending", "0/1 nodes available", "insufficient cpu", "insufficient memory"}, "Pod stuck in Pending state — cannot be scheduled", "Check node resources with `kubectl describe pod`; add nodes or reduce requests", "high"},
	{"k8s-image-pull", "kubernetes", []string{"errimagepull", "imagepullbackoff", "failed to pull image"}, "Pod cannot pull container image", "Verify image name and tag; check imagePullSecrets for private registries", "high"},
	{"k8s-liveness-fail", "kubernetes", []string{"liveness probe failed", "liveness probe", "unhealthy: liveness"}, "Liveness probe is failing — pod will be restarted", "Check liveness probe path and port; increase initialDelaySeconds if app starts slowly", "high"},
	{"k8s-readiness-fail", "kubernetes", []string{"readiness probe failed", "readiness probe", "unhealthy: readiness"}, "Readiness probe failing — pod not serving traffic", "Verify application is ready to serve; check readiness endpoint health", "high"},
	{"k8s-node-not-ready", "kubernetes", []string{"node not ready", "nodenotready", "node condition"}, "Kubernetes node is NotReady", "Run `kubectl describe node`; check kubelet logs on the node", "critical"},
	{"k8s-evicted", "kubernetes", []string{"pod evicted", "low disk space", "nodefs", "imagefs"}, "Pod evicted due to resource pressure", "Free disk space on nodes; increase PVC sizes; set appropriate resource limits", "high"},
	{"k8s-pvc-pending", "kubernetes", []string{"pvc pending", "persistentvolumeclaim pending", "no persistent volumes available"}, "PersistentVolumeClaim stuck in Pending", "Check StorageClass exists; verify PV capacity and access modes match", "high"},
	{"k8s-configmap-missing", "kubernetes", []string{"configmap not found", "configmap.*does not exist", "failed to get configmap"}, "Required ConfigMap is missing", "Create the ConfigMap: `kubectl create configmap ...`", "high"},
	{"k8s-secret-missing", "kubernetes", []string{"secret not found", "secret.*does not exist", "failed to get secret"}, "Required Secret is missing", "Create the Secret: `kubectl create secret ...`", "critical"},
	{"k8s-rbac-denied", "kubernetes", []string{"forbidden: user", "rbac", "cannot get", "cannot list", "unauthorized kubernetes"}, "RBAC permission denied in Kubernetes", "Add required RBAC rules to the ServiceAccount's ClusterRole/Role", "high"},
	{"k8s-hpa-maxed", "kubernetes", []string{"hpa max replicas", "maxreplicas reached", "horizontal pod autoscaler"}, "HPA has reached maximum replica count", "Increase HPA maxReplicas or optimize application to reduce per-pod load", "high"},
	{"k8s-dns-fail", "kubernetes", []string{"dns resolution failed kubernetes", "coredns error", "servfail", "nxdomain kubernetes"}, "Kubernetes cluster DNS failure", "Check CoreDNS pods: `kubectl get pods -n kube-system -l k8s-app=kube-dns`", "critical"},
	{"k8s-network-policy", "kubernetes", []string{"networkpolicy", "traffic dropped", "connection refused kubernetes", "egress blocked"}, "NetworkPolicy blocking expected traffic", "Review NetworkPolicy rules; use `kubectl exec` to test connectivity", "high"},
	{"k8s-ingress-error", "kubernetes", []string{"ingress error", "nginx ingress", "502 bad gateway ingress", "ingress controller"}, "Ingress controller error", "Check ingress controller logs; verify service and pod health behind the ingress", "high"},
	{"k8s-resource-quota", "kubernetes", []string{"exceeded quota", "resource quota", "namespace quota"}, "Namespace resource quota exceeded", "Increase quota with `kubectl edit resourcequota` or free unused resources", "high"},
	{"k8s-affinity-fail", "kubernetes", []string{"node affinity", "pod affinity", "no nodes match affinity", "taint"}, "Pod cannot be scheduled due to affinity/taint rules", "Review pod's nodeAffinity and tolerations; verify node labels", "high"},
	{"k8s-statefulset-stuck", "kubernetes", []string{"statefulset stuck", "statefulset rollout stuck", "statefulset pod"}, "StatefulSet rollout is stuck", "Check pod status with `kubectl describe`; verify PVC binding", "high"},
	{"k8s-deployment-stuck", "kubernetes", []string{"deployment stuck", "rollout stuck", "deployment not progressing"}, "Deployment rollout is not progressing", "Run `kubectl rollout status deployment`; check for failing pods in new ReplicaSet", "high"},
	{"k8s-job-failed", "kubernetes", []string{"job failed", "backofflimitexceeded", "job pod failed"}, "Kubernetes Job exceeded backoff limit", "Check Job pod logs for the error; increase backoffLimit or fix the Job spec", "high"},
	{"k8s-cert-expired", "kubernetes", []string{"certificate expired kubernetes", "tls certificate expired", "x509: certificate has expired"}, "TLS certificate expired in Kubernetes", "Renew certificate; use cert-manager for automated certificate rotation", "critical"},
	{"k8s-etcd-error", "kubernetes", []string{"etcd error", "etcd cluster", "etcdserver"}, "etcd cluster error (Kubernetes control plane)", "Check etcd pod health: `kubectl get pods -n kube-system -l component=etcd`", "critical"},
	{"k8s-api-server-down", "kubernetes", []string{"unable to connect to server", "api server unreachable", "kube-apiserver"}, "Kubernetes API server unreachable", "Check API server pods in kube-system namespace; verify control plane node health", "critical"},
	{"k8s-kubelet-error", "kubernetes", []string{"kubelet error", "kubelet exited", "failed to run kubelet"}, "Kubelet process error on node", "SSH to node and check: `systemctl status kubelet`; review kubelet logs", "critical"},
	{"k8s-webhook-error", "kubernetes", []string{"admission webhook", "webhook denied", "validating webhook"}, "Admission webhook blocking resource creation", "Check webhook configuration; look at webhook server logs for rejection reason", "high"},
	{"k8s-storage-class", "kubernetes", []string{"storageclass not found", "no storageclass", "storageclass error"}, "StorageClass missing or misconfigured", "Verify StorageClass exists: `kubectl get storageclass`", "high"},
	{"k8s-pod-disruption", "kubernetes", []string{"pod disruption budget", "pdb", "disruption budget violated"}, "Pod Disruption Budget violation", "Review PDB minAvailable/maxUnavailable settings; scale deployment before disruption", "medium"},
	{"k8s-container-runtime", "kubernetes", []string{"container runtime", "docker error", "containerd error", "cri error"}, "Container runtime error on Kubernetes node", "Check containerd/docker service on the node; look for storage/disk issues", "high"},
	{"k8s-metrics-server", "kubernetes", []string{"metrics server", "unable to fetch metrics", "heapster"}, "Kubernetes metrics-server unavailable", "Deploy metrics-server: check pod health in kube-system namespace", "medium"},
	{"k8s-init-container", "kubernetes", []string{"init container failed", "init:error", "initcontainer"}, "Init container failed preventing pod start", "Check init container logs: `kubectl logs <pod> -c <init-container>`", "high"},
	{"k8s-ephemeral-storage", "kubernetes", []string{"ephemeral storage", "local ephemeral storage", "ephemeral-storage"}, "Pod exceeded ephemeral storage limit", "Increase ephemeral-storage limit; mount PVC instead of using node storage", "high"},
	{"k8s-namespace-terminating", "kubernetes", []string{"namespace terminating", "namespace stuck terminating", "finalizer"}, "Kubernetes namespace stuck in Terminating state", "Remove stuck finalizers: `kubectl edit namespace <ns>` and delete finalizers array", "medium"},
	{"k8s-service-unavailable", "kubernetes", []string{"service unavailable kubernetes", "no endpoints", "endpoints not ready"}, "No ready endpoints behind Kubernetes Service", "Check pod readiness: `kubectl get endpoints <svc>`; fix failing readiness probes", "critical"},
	{"k8s-daemonset-error", "kubernetes", []string{"daemonset error", "daemonset pod failed", "daemonset not scheduled"}, "DaemonSet pod scheduling error", "Check node taints; verify DaemonSet tolerations match node taints", "high"},
	{"k8s-cni-error", "kubernetes", []string{"cni error", "network plugin", "failed to set up pod network", "calico error", "flannel error"}, "Container Network Interface (CNI) plugin error", "Check CNI pod health in kube-system; verify node network configuration", "critical"},
	{"k8s-volumemount-error", "kubernetes", []string{"volume mount error", "failed to mount volume", "volumemount"}, "Volume mount failure in pod", "Check PVC is bound; verify volume path and permissions", "high"},
	{"k8s-env-injection", "kubernetes", []string{"env var injection", "envfrom configmap", "envfrom secret"}, "Environment variable injection failed", "Verify referenced ConfigMap/Secret exists in the same namespace", "high"},
	{"k8s-cronjob-missed", "kubernetes", []string{"cronjob missed", "startingdeadlineseconds", "cronjob failed to schedule"}, "Kubernetes CronJob missed scheduled execution", "Increase startingDeadlineSeconds; check cluster time synchronization", "medium"},
	{"k8s-topology-spread", "kubernetes", []string{"topology spread constraint", "topologykey", "spread constraint"}, "Pod topology spread constraint cannot be satisfied", "Relax spread constraints or add nodes matching the topologyKey", "medium"},

	// ══════════════════════════════════════════════════
	// GENERIC / INFRASTRUCTURE (30 patterns)
	// ══════════════════════════════════════════════════
	{"infra-disk-full", "infrastructure", []string{"no space left on device", "disk full", "disk space", "filesystem full"}, "Disk is full", "Free disk space immediately; expand volume or archive old data", "critical"},
	{"infra-cpu-spike", "infrastructure", []string{"cpu usage", "cpu spike", "high cpu", "100% cpu", "load average"}, "CPU usage spike detected", "Profile the process consuming CPU; scale horizontally if load is legitimate", "high"},
	{"infra-memory-pressure", "infrastructure", []string{"memory pressure", "swap usage", "oom score", "available memory low"}, "System memory pressure", "Identify memory-heavy processes; add RAM or reduce memory limits", "high"},
	{"infra-network-latency", "infrastructure", []string{"network latency", "high latency", "packet loss", "ping timeout"}, "Network latency or packet loss detected", "Check network path with `traceroute`; investigate switch/router health", "high"},
	{"infra-service-down", "infrastructure", []string{"service is down", "service unavailable", "service not responding", "health check failed"}, "Service health check failed", "Restart service; check error logs for crash reason", "critical"},
	{"infra-certificate-expiry", "infrastructure", []string{"certificate expires", "certificate expiry", "ssl expiry", "tls expiry"}, "TLS certificate expiring or expired", "Renew certificate immediately; set up automated renewal with certbot/cert-manager", "critical"},
	{"infra-load-balancer", "infrastructure", []string{"load balancer error", "backend unhealthy", "502 bad gateway", "503 service unavailable"}, "Load balancer reporting unhealthy backends", "Check all backend instances; remove unhealthy ones from rotation", "critical"},
	{"infra-dns-failure", "infrastructure", []string{"dns failure", "dns resolution error", "name resolution failed", "nslookup failed"}, "DNS resolution failure", "Check DNS server configuration; verify /etc/resolv.conf", "high"},
	{"infra-firewall", "infrastructure", []string{"connection blocked", "firewall drop", "iptables drop", "security group"}, "Firewall rule blocking traffic", "Review security group/iptables rules; allow required port", "high"},
	{"infra-ntp-drift", "infrastructure", []string{"ntp drift", "clock skew", "time synchronization"}, "System clock drift detected", "Sync with NTP: `ntpdate -u pool.ntp.org`; check chronyd/ntpd service", "medium"},
	{"infra-auth-failure", "infrastructure", []string{"authentication failed", "invalid credentials", "unauthorized", "auth failure"}, "Authentication failure", "Verify credentials; check for expired passwords or rotated API keys", "high"},
	{"infra-rate-limit", "infrastructure", []string{"rate limited", "429", "throttled", "too many requests"}, "Rate limit exceeded", "Implement exponential backoff; review rate limit configuration", "medium"},
	{"infra-config-error", "infrastructure", []string{"configuration error", "invalid config", "config parse error"}, "Invalid configuration", "Validate configuration file syntax; review recent config changes", "high"},
	{"infra-dependency-down", "infrastructure", []string{"dependency unavailable", "upstream service down", "downstream failed"}, "Upstream dependency is unavailable", "Check dependency health; implement circuit breaker pattern", "high"},
	{"infra-deployment-failed", "infrastructure", []string{"deployment failed", "rollout failed", "deploy failed"}, "Deployment failed", "Review deployment logs; roll back to last known good version", "critical"},
	{"infra-backup-failed", "infrastructure", []string{"backup failed", "snapshot failed", "backup error"}, "Backup job failed", "Check storage availability and credentials; alert if backup hasn't run in > 24h", "high"},
	{"infra-ssh-failure", "infrastructure", []string{"ssh connection refused", "ssh timeout", "permission denied publickey"}, "SSH connection failure", "Verify SSH service is running; check authorized_keys and firewall rules", "high"},
	{"infra-queue-depth", "infrastructure", []string{"queue depth", "message queue full", "backlog growing"}, "Message queue backlog growing", "Scale consumers; investigate slow processing; check consumer health", "high"},
	{"infra-gc-pause", "infrastructure", []string{"gc pause", "garbage collection pause", "stop the world"}, "Long GC pause causing latency", "Tune GC settings; increase heap size; reduce object allocation", "medium"},
	{"infra-swap-usage", "infrastructure", []string{"swap usage", "swap space", "swapping", "swap in"}, "System using swap space", "Add RAM; reduce process memory usage; set swappiness to 0 for databases", "high"},
	{"infra-kernel-panic", "infrastructure", []string{"kernel panic", "oops kernel", "system panic"}, "Kernel panic — host rebooted unexpectedly", "Check kernel logs with `dmesg`; investigate hardware issues", "critical"},
	{"infra-io-wait", "infrastructure", []string{"io wait", "disk io", "high iowait", "iops limit"}, "High I/O wait — disk throughput saturated", "Profile with `iostat`; optimize queries or upgrade storage", "high"},
	{"infra-open-files", "infrastructure", []string{"too many open files", "file descriptor limit", "ulimit"}, "File descriptor limit reached", "Increase ulimit; audit and close unused file descriptors", "high"},
	{"infra-zombie-process", "infrastructure", []string{"zombie process", "defunct process", "dead process"}, "Zombie processes accumulating", "Find and kill the parent process; review process lifecycle management", "medium"},
	{"infra-docker-error", "infrastructure", []string{"docker error", "container failed", "docker daemon"}, "Docker container or daemon error", "Check `docker logs`; verify daemon health with `systemctl status docker`", "high"},
	{"infra-vpn-down", "infrastructure", []string{"vpn down", "vpn disconnected", "tunnel down"}, "VPN tunnel is down", "Reconnect VPN; check VPN server health and credentials", "high"},
	{"infra-healthcheck-fail", "infrastructure", []string{"health check failed", "healthcheck failing", "/health 500"}, "Health check endpoint returning errors", "Investigate what the health check is testing; fix the underlying issue", "high"},
	{"infra-port-conflict", "infrastructure", []string{"address already in use", "port already bound", "bind: address already in use"}, "Port conflict — address already in use", "Identify the process using the port: `lsof -i :<port>`; stop or reconfigure it", "high"},
	{"infra-timezone-error", "infrastructure", []string{"timezone error", "invalid timezone", "time zone mismatch"}, "Timezone configuration error", "Set TZ environment variable; verify /etc/localtime symlink", "low"},
	{"infra-api-gateway", "infrastructure", []string{"api gateway error", "api gateway timeout", "gateway 504"}, "API gateway error or timeout", "Check backend service health; review gateway timeout and retry configuration", "high"},
}

// ─────────────────────────────────────────────────────
// Matching
// ─────────────────────────────────────────────────────

// MatchSignatures returns all signatures whose keywords appear in text.
// text should be a concatenation of relevant incident fields (title, message,
// root_cause_summary, service name, etc.) — all lowercased.
func MatchSignatures(text string) []Signature {
	lower := strings.ToLower(text)
	var matched []Signature
	for _, sig := range Library {
		if matchesSig(lower, sig) {
			matched = append(matched, sig)
		}
	}
	return matched
}

// MatchSignaturesByCategory limits matching to a specific technology category.
func MatchSignaturesByCategory(text, category string) []Signature {
	lower := strings.ToLower(text)
	var matched []Signature
	for _, sig := range Library {
		if sig.Category == category && matchesSig(lower, sig) {
			matched = append(matched, sig)
		}
	}
	return matched
}

func matchesSig(lowerText string, sig Signature) bool {
	for _, kw := range sig.Keywords {
		if strings.Contains(lowerText, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// SignatureCount returns the total number of patterns in the library.
func SignatureCount() int {
	return len(Library)
}
