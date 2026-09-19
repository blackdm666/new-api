"""CI-only real RC37 -> RC38 startup rehearsal; never connects to production."""

import json
import os
from pathlib import Path
import sqlite3
import subprocess
import tempfile
import time
import urllib.request


def run(args, data=None):
    return subprocess.run(
        args, input=data, capture_output=True, check=True, timeout=180
    ).stdout.decode().strip()


def main():
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise RuntimeError("This rehearsal is restricted to disposable GitHub runners")
    root = Path.cwd()
    old_image = os.environ.get("MIGRATION_BASE_IMAGE", "ghcr.io/blackdm666/new-api:88api-68b49b0")
    baseline = os.environ.get("MIGRATION_BASE_LABEL", "rc37")
    current = root / "new-api-upgrade-test"
    run(["go", "build", "-o", str(current), "."])
    run(["docker", "pull", old_image])
    container = run(["docker", "create", old_image])
    reports = {}
    try:
        with tempfile.TemporaryDirectory(prefix="newapi-rc38-") as directory:
            work = Path(directory)
            old = work / "old-new-api"
            run(["docker", "cp", container + ":/new-api", str(old)])
            old.chmod(0o700)
            services = {}
            for dialect, image in [("mysql", "mysql:8.0"), ("postgres", "postgres:16")]:
                ids = run(["docker", "ps", "-q", "--filter", "ancestor=" + image]).splitlines()
                if len(ids) != 1:
                    raise RuntimeError("Expected one disposable " + dialect + " service")
                services[dialect] = ids[0]

            for dialect in ["sqlite", "mysql", "postgres"]:
                database = work / (dialect + ".db")

                def query(sql):
                    if dialect == "sqlite":
                        with sqlite3.connect(database) as connection:
                            rows = connection.execute(sql).fetchall()
                        return "\n".join("\t".join(str(value) for value in row) for row in rows)
                    if dialect == "mysql":
                        return run([
                            "docker", "exec", "-i", "-e", "MYSQL_PWD=fixture-only",
                            services[dialect], "mysql", "-uroot", "-N", "-B", "rc38_upgrade",
                        ], sql.encode())
                    return run([
                        "docker", "exec", "-i", services[dialect], "psql",
                        "-U", "postgres", "-d", "rc38_upgrade", "-At", "-F", "\t",
                    ], sql.encode())

                if dialect == "mysql":
                    run(["docker", "exec", "-e", "MYSQL_PWD=fixture-only", services[dialect],
                         "mysql", "-uroot", "-e", "CREATE DATABASE rc38_upgrade;"])
                elif dialect == "postgres":
                    run(["docker", "exec", services[dialect], "createdb", "-U", "postgres", "rc38_upgrade"])

                env = os.environ.copy()
                env.update({
                    "PORT": "19338", "SESSION_SECRET": "disposable-session-fixture-only",
                    "CRYPTO_SECRET": "disposable-crypto-fixture-only",
                    "SQLITE_PATH": str(database), "BATCH_UPDATE_ENABLED": "false",
                    "TASK_VIDEO_CACHE_ENABLED": "false", "TASK_VIDEO_WORKER_ENABLED": "false",
                    "SHUTDOWN_TIMEOUT_SECONDS": "2", "SQL_DSN": "", "LOG_SQL_DSN": "",
                    "REDIS_CONN_STRING": "",
                })
                if dialect == "mysql":
                    env["SQL_DSN"] = "root:fixture-only@tcp(127.0.0.1:3306)/rc38_upgrade?charset=utf8mb4&parseTime=True&loc=Local"
                elif dialect == "postgres":
                    env["SQL_DSN"] = "postgres://postgres:fixture-only@127.0.0.1:5432/rc38_upgrade?sslmode=disable"
                stages = []
                schemas = []
                for stage, binary in [(baseline, old), ("candidate", current), ("candidate-repeat", current)]:
                    logfile = work / (dialect + "-" + stage + ".log")
                    with logfile.open("wb") as output:
                        process = subprocess.Popen([str(binary)], cwd=work, env=env, stdout=output, stderr=output)
                        try:
                            deadline = time.monotonic() + 120
                            while time.monotonic() < deadline:
                                if process.poll() is not None:
                                    raise RuntimeError(logfile.read_text()[-12000:])
                                try:
                                    with urllib.request.urlopen("http://127.0.0.1:19338/api/status", timeout=2) as response:
                                        if json.load(response).get("success"):
                                            break
                                except (OSError, ValueError):
                                    pass
                                time.sleep(0.5)
                            else:
                                raise RuntimeError("Startup timeout: " + dialect + "/" + stage)
                            key = "`key`" if dialect == "mysql" else '"key"'
                            if stage == baseline:
                                query(f"INSERT INTO options ({key}, value) VALUES ('RC38MigrationSentinel', 'preserve-custom-settings');")
                                query("INSERT INTO email_deliveries (delivery_key, category, recipient, subject, body, priority, state, next_attempt_time, created_time, updated_time) VALUES ('notice-upgrade-sentinel', 'system_alert', 'fixture@example.invalid', 'Preserved subject', 'Preserved body', 100, 'queued', 4102444800, 1, 1);")
                            if query(f"SELECT value FROM options WHERE {key}='RC38MigrationSentinel';") != "preserve-custom-settings":
                                raise RuntimeError("Existing option data lost")
                            if query("SELECT body FROM email_deliveries WHERE delivery_key='notice-upgrade-sentinel';") != "Preserved body":
                                raise RuntimeError("Existing outbox data lost")
                            if stage == "candidate":
                                query("INSERT INTO user_notices (kind, subject, body, status, recipient_data, created_by, updated_by, created_time, updated_time) VALUES ('maintenance', 'Migration draft', 'Keep this draft', 'draft', '[]', 1, 1, 1, 1);")
                            if stage != baseline and query("SELECT body FROM user_notices WHERE subject='Migration draft';") != "Keep this draft":
                                raise RuntimeError("Notification draft lost on repeated migration")
                            if dialect == "sqlite":
                                schema = query("SELECT name, sql FROM sqlite_master WHERE type IN ('table','index') AND sql IS NOT NULL ORDER BY name;")
                            elif dialect == "mysql":
                                schema = query("SELECT TABLE_NAME,COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COALESCE(COLUMN_DEFAULT,'NULL') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='rc38_upgrade' ORDER BY TABLE_NAME,ORDINAL_POSITION;")
                            else:
                                schema = query("SELECT table_name,column_name,data_type,is_nullable,COALESCE(column_default,'NULL') FROM information_schema.columns WHERE table_schema='public' ORDER BY table_name,ordinal_position;")
                            schemas.append(schema)
                            stages.append(stage + ":healthy")
                        finally:
                            process.terminate()
                            try:
                                process.wait(timeout=20)
                            except subprocess.TimeoutExpired:
                                process.kill()
                                process.wait()
                                raise RuntimeError("Application did not stop gracefully")
                    if process.returncode != 0:
                        raise RuntimeError("Unexpected shutdown status " + str(process.returncode))
                if schemas[1] != schemas[2]:
                    raise RuntimeError("Repeated startup changed schema: " + dialect)
                reports[dialect] = {"baseline_image": old_image, "stages": stages, "custom_option_preserved": True, "outbox_preserved": True, "notification_draft_preserved": True, "repeated_schema_stable": True}
    finally:
        run(["docker", "rm", container])
        current.unlink(missing_ok=True)
    print(json.dumps(reports, indent=2))


if __name__ == "__main__":
    main()
