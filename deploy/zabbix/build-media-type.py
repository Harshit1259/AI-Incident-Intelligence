#!/usr/bin/env python3
"""Build the importable Zabbix media type from neuroops-webhook.js.

Writes neuroops-zabbix.yaml (Zabbix 6.0 export format; Zabbix 7.0 imports it
too) next to this script and copies it to frontend/public/integrations/ so the
Integrations page can offer it as a download.

    python3 deploy/zabbix/build-media-type.py
"""
import os
import shutil

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))

# (parameter name, value) — the script reads these names. Macros are expanded
# by Zabbix before the script runs.
PARAMETERS = [
    ("url", "https://neuroops.example.com/api/v1/ingest/zabbix"),
    ("token", "<your source token>"),
    ("http_proxy", ""),
    ("zabbix_url", "{$ZABBIX.URL}"),
    ("event_id", "{EVENT.ID}"),
    ("event_value", "{EVENT.VALUE}"),
    ("event_status", "{EVENT.STATUS}"),
    ("event_name", "{EVENT.NAME}"),
    ("event_nseverity", "{EVENT.NSEVERITY}"),
    ("event_opdata", "{EVENT.OPDATA}"),
    ("event_date", "{EVENT.DATE}"),
    ("event_time", "{EVENT.TIME}"),
    ("event_tags_json", "{EVENT.TAGSJSON}"),
    ("event_update_status", "{EVENT.UPDATE.STATUS}"),
    ("event_update_action", "{EVENT.UPDATE.ACTION}"),
    ("event_update_message", "{EVENT.UPDATE.MESSAGE}"),
    ("event_update_date", "{EVENT.UPDATE.DATE}"),
    ("event_update_time", "{EVENT.UPDATE.TIME}"),
    ("user_fullname", "{USER.FULLNAME}"),
    ("trigger_id", "{TRIGGER.ID}"),
    ("trigger_name", "{TRIGGER.NAME}"),
    ("trigger_hostgroup_name", "{TRIGGER.HOSTGROUP.NAME}"),
    ("host_host", "{HOST.HOST}"),
    ("host_name", "{HOST.NAME}"),
    ("host_ip", "{HOST.IP}"),
    ("item_lastvalue", "{ITEM.LASTVALUE1}"),
]

DESCRIPTION = """Sends Zabbix problems, recoveries and acknowledgements to NeuroOps.

1. Set "url" to your NeuroOps Zabbix endpoint and "token" to a NeuroOps source token
   (NeuroOps → Integrations → Zabbix → Connect).
2. Optional: set the global macro {$ZABBIX.URL} to your Zabbix frontend URL for
   "Open in Zabbix" links.
3. Add this media type to a dedicated user (e.g. "neuroops") with read access to
   the host groups to send — not a person's account: Zabbix never sends update
   notifications to the user who made the update.
4. Create a trigger action with Operations, Recovery operations and Update
   operations sending to that user via NeuroOps."""

MESSAGES = [
    ("PROBLEM", "Problem: {EVENT.NAME}", "Problem: {EVENT.NAME} on {HOST.NAME}"),
    ("RECOVERY", "Resolved: {EVENT.NAME}", "Resolved: {EVENT.NAME} on {HOST.NAME}"),
    ("UPDATE", "Updated: {EVENT.NAME}", "{USER.FULLNAME} {EVENT.UPDATE.ACTION} problem {EVENT.NAME}"),
]


def q(s):
    """YAML single-quoted scalar."""
    return "'" + s.replace("'", "''") + "'"


def block(text, indent):
    pad = " " * indent
    return "\n".join((pad + line) if line else "" for line in text.rstrip("\n").split("\n"))


def build():
    with open(os.path.join(HERE, "neuroops-webhook.js")) as f:
        script = f.read()

    out = ["zabbix_export:", "  version: '6.0'", "  media_types:", "    - name: NeuroOps", "      type: WEBHOOK", "      parameters:"]
    for name, value in PARAMETERS:
        out += [f"        - name: {name}", f"          value: {q(value)}"]
    out += ["      script: |", block(script, 8), "      timeout: 10s", "      process_tags: 'NO'",
            "      show_event_menu: 'NO'", "      description: |", block(DESCRIPTION, 8), "      message_templates:"]
    for mode, subject, message in MESSAGES:
        out += ["        - event_source: TRIGGERS", f"          operation_mode: {mode}",
                f"          subject: {q(subject)}", f"          message: {q(message)}"]
    return "\n".join(out) + "\n"


if __name__ == "__main__":
    yaml = build()
    target = os.path.join(HERE, "neuroops-zabbix.yaml")
    with open(target, "w") as f:
        f.write(yaml)
    public = os.path.join(ROOT, "frontend", "public", "integrations")
    os.makedirs(public, exist_ok=True)
    shutil.copy(target, os.path.join(public, "neuroops-zabbix.yaml"))
    print("wrote", target, "and", os.path.join(public, "neuroops-zabbix.yaml"))
