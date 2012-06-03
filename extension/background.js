var ws = null;

var Server = function(label, host) {
    this.label = label;
    this.host = host;
    this.status = 'connecting';
    this.nextRetryTime = 0;
};

Server.prototype.getMessage = function() {
    var msg = this.getStatusMessage();
    msg['label'] = this.label;
    msg['host'] = this.host;
    return msg;
};

Server.prototype.getStatusMessage = function() {
    var msg = {};
    msg['status'] = this.status;
    var now = Date.now();
    if (this.nextRetryTime > now) {
        msg['retry_sec'] = Math.floor((this.nextRetryTime - now) / 1000);
    }
    return msg;
};

var servers = [
    new Server('Ubuntu', 'localhost:8888', 'connecting')
];

function sendMessage() {
    var input = document.getElementById('input').value;
    if (!input) {
        return;
    }
    document.getElementById('input').value = '';
    ws.send(input);
}

var retryIntervalMin = 1000;  // ms
var retryIntervalMax = 60 * 1000;
var retryInterval = retryIntervalMin;

function connectToServer() {
    console.log('connectToServer');
    var url = 'ws://' + servers[0].host + '/ws';
    ws = new WebSocket(url);
    ws.onmessage = onmessage;
    ws.onopen = onopen;
    ws.onclose = onclose;
    ws.onerror = onerror;
}

var configPorts = [];

function onConnectConfig(port) {
    configPorts.push(port);
    port.postMessage({'reload': [servers[0].getMessage()]});
    port.onDisconnect.addListener(onDisconnectConfig);
}

function onDisconnectConfig(port) {
    for (var i = 0; i < configPorts.length; ++i) {
        if (port == configPorts[i]) {
            configPorts.splice(i, 1);
            break;
        }
    }
}

function init() {
    connectToServer();
    chrome.extension.onConnect.addListener(onConnectConfig);
}

function addMessage(message) {
    console.log(message);
}

function onerror(e) {
    addMessage('Encountered error.');
    addMessage(e)
}

var connectionOpened = false;

function onopen(e) {
    servers[0].status = 'connected';
    for (var i = 0; i < configPorts.length; ++i) {
        configPorts[i].postMessage({'update': [{'status': 'connected'}]});
    }
    connectionOpened = true;
    retryInterval = retryIntervalMin;
    var popup = webkitNotifications.createNotification(
        "",  // no icon. Don't use null.
        "Notification",
        "Connected to the werver.");
    setTimeout(function() { popup.cancel(); }, 5000);
    popup.show();
}

function onclose(e) {
    if (connectionOpened) {
        var popup = webkitNotifications.createNotification(
            "",  // no icon. Don't use null.
            "Notification",
            "The connection was closed by the server.");
        setTimeout(function() { popup.cancel(); }, 5000);
        popup.show();
    } else {
        console.log('Failed to connect to the server.');
    }
    connectionOpened = false;
    setTimeout(connectToServer, retryInterval);
    servers[0].status = 'connecting';
    servers[0].nextRetryTime = Date.now() + retryInterval;
    for (var i = 0; i < configPorts.length; ++i) {
        configPorts[i].postMessage({'update': [servers[0].getStatusMessage()]});
    }
    retryInterval = retryInterval * 2;
    if (retryInterval > retryIntervalMax) {
        retryInterval = retryIntervalMax;
    }
}

function onmessage(e) {
    addMessage('Recieved: ' + e.data);
    var obj = JSON.parse(e.data);
    var id = obj['Id']
    addMessage('Id: ' + id)
    var path = obj['OpenUrl']
    addMessage('url: ' + path)
    if (path) {
        var url = 'http://' + servers[0].host + '/fwd/' + id + path;
        addMessage(url)
        addMessage(chrome)
        addMessage(chrome.tabs)
        chrome.tabs.create({'url': url, 'selected': true});
    }
    var notification = obj['Notification']
    if (notification) {
        var popup = webkitNotifications.createNotification(
            "",  // no icon. Don't use null.
            "Notification",
            notification);
        setTimeout(function() { popup.cancel(); }, 5000);
        popup.show();
    }
    var closeTabs = obj['CloseTabs']
    if (closeTabs) {
        chrome.tabs.getAllInWindow(null, function(tabs) {
            for (var i = 0; i < tabs.length; ++i) {
                if (tabs[i].url.indexOf(String(id)) != -1) {
                    chrome.tabs.remove(tabs[i].id);
                }
            }
        });
    }
}
