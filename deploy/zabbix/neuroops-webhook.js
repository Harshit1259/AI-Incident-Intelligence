// NeuroOps webhook for Zabbix (media type script).
//
// Zabbix runs this for every problem, recovery and update an action sends to
// the NeuroOps media type. `value` is a JSON string of the media type's
// parameters, with Zabbix macros already expanded. The script sends one JSON
// object to NeuroOps (POST /api/v1/ingest/zabbix).
//
// Source of truth: deploy/zabbix/neuroops-webhook.js — the same script is
// embedded in neuroops-zabbix.yaml (regenerate with build-media-type.py).
try {
    var params = JSON.parse(value);

    if (typeof params.url !== 'string' || params.url.indexOf('http') !== 0) {
        throw 'set the "url" parameter to your NeuroOps Zabbix endpoint';
    }
    if (typeof params.token !== 'string' || params.token === '' || params.token.charAt(0) === '<') {
        throw 'set the "token" parameter to your NeuroOps source token';
    }

    // A macro Zabbix could not expand is left as "{MACRO}"; send "" instead.
    var clean = function (v) {
        if (typeof v !== 'string') {
            return '';
        }
        return (v.charAt(0) === '{' && v.charAt(v.length - 1) === '}') ? '' : v;
    };

    var tags = [];
    var tagsJSON = clean(params.event_tags_json);
    if (tagsJSON.charAt(0) === '[') {
        try {
            tags = JSON.parse(tagsJSON);
        } catch (e) {
            tags = [];
        }
    }

    var isUpdate = params.event_update_status === '1';

    var zabbixURL = '';
    var base = clean(params.zabbix_url);
    if (base !== '') {
        zabbixURL = base.replace(/\/+$/, '') + '/tr_events.php?triggerid=' +
            encodeURIComponent(params.trigger_id) + '&eventid=' + encodeURIComponent(params.event_id);
    }

    var payload = {
        event_id: params.event_id,
        event_value: params.event_value,
        event_status: params.event_status,
        update_status: isUpdate ? '1' : '0',
        update_action: isUpdate ? clean(params.event_update_action) : '',
        update_message: isUpdate ? clean(params.event_update_message) : '',
        update_user: isUpdate ? clean(params.user_fullname) : '',
        trigger_id: params.trigger_id,
        trigger_name: clean(params.trigger_name),
        event_name: clean(params.event_name),
        severity: clean(params.event_nseverity),
        host: clean(params.host_host),
        host_name: clean(params.host_name),
        host_ip: clean(params.host_ip),
        host_group: clean(params.trigger_hostgroup_name),
        tags: tags,
        item_value: clean(params.item_lastvalue),
        opdata: clean(params.event_opdata),
        event_time: (clean(params.event_date) + ' ' + clean(params.event_time)).trim(),
        zabbix_url: zabbixURL
    };

    // Zabbix retries failed sends; the key makes a retry a no-op in NeuroOps.
    // Updates are keyed by their own time so each acknowledgement counts once.
    var key = 'zbx-' + params.event_id + '-' + (isUpdate
        ? 'u-' + clean(params.event_update_date) + '-' + clean(params.event_update_time)
        : params.event_value);

    var req = new HttpRequest();
    if (typeof params.http_proxy === 'string' && params.http_proxy.trim() !== '') {
        req.setProxy(params.http_proxy.trim());
    }
    req.addHeader('Content-Type: application/json');
    req.addHeader('Authorization: Bearer ' + params.token);
    req.addHeader('X-Idempotency-Key: ' + key);

    var resp = req.post(params.url, JSON.stringify(payload));
    var status = req.getStatus();
    if (status < 200 || status >= 300) {
        throw 'NeuroOps returned HTTP ' + status + ': ' + resp;
    }
    return 'OK';
} catch (error) {
    Zabbix.log(3, '[NeuroOps webhook] ' + error);
    throw 'NeuroOps webhook failed: ' + error;
}
