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

var servers = [];

function sendMessage() {
    var input = document.getElementById('input').value;
    if (!input) {
        return;
    }
    document.getElementById('input').value = '';
    ws.send(input);
}

var retryIntervalMin = 10 * 1000;  // ms
var retryIntervalMax = 10 * 60 * 1000;
var retryInterval = retryIntervalMin;

function connectToServer() {
    // clear existing timer to avoid we create multi-timers.
    clearTimeout(retryTimerId);
    retryTimerId = null;
    if (ws) {
        return;
    }
    console.log('connectToServer');
    var url = 'ws://' + servers[0].host + '/ws';
    ws = new WebSocket(url);
    ws.onmessage = onmessage;
    ws.onopen = onopen;
    ws.onclose = onclose;
    ws.onerror = onerror;
}

function disconnectFromServer() {
    if (!ws) {
        clearTimeout(retryTimerId);
        retryInterval = retryIntervalMin;
        servers[0].status = 'disconnected';
        for (var i = 0; i < configPorts.length; ++i) {
            configPorts[i].postMessage(
                {'update': [servers[0].getStatusMessage()]});
        }
        return;
    }
    ws.close();
    ws = null;
}

var configPorts = [];

function onConnectConfig(port) {
    configPorts.push(port);
    port.postMessage({'reload': [servers[0].getMessage()]});
    port.onDisconnect.addListener(onDisconnectConfig);
    port.onMessage.addListener(onMessageFromConfig);
}

function onDisconnectConfig(port) {
    for (var i = 0; i < configPorts.length; ++i) {
        if (port == configPorts[i]) {
            configPorts.splice(i, 1);
            break;
        }
    }
}

function onMessageFromConfig(msg, port) {
    if (msg['connect']) {
        connectToServer();
    } else if (msg['disconnect']) {
        disconnectFromServer();
    } else if (msg['reload']) {
        var settings = msg['reload'];
        console.log(settings);
        localStorage['server-settings'] = JSON.stringify(settings);
        disconnectFromServer();
        setUpServers(settings);
        connectToServer();
    }
}

function setUpServers(settings) {
    servers = [];
    if (settings.length == 0) {
        // We need at least one server.
        // TODO: Remove this hack.
        settings.push({'label': 'MyServer', 'host': 'localhost:8888'});
    }
    for (var i = 0; i < settings.length; ++i) {
        servers.push(new Server(
            settings[i]['label'], settings[i]['host'], 'connecting'));
    }
    for (var i = 0; i < configPorts.length; ++i) {
        configPorts[i].postMessage({'reload': [servers[0].getMessage()]});
    }
}

function init() {
    var settings;
    try {
        settings = JSON.parse(localStorage['server-settings']);
    } catch (e) {
        settings = [];
    }
    setUpServers(settings);
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
var retryTimerId = null;

function onopen(e) {
    servers[0].status = 'connected';
    for (var i = 0; i < configPorts.length; ++i) {
        configPorts[i].postMessage({'update': [{'status': 'connected'}]});
    }
    connectionOpened = true;
    retryInterval = retryIntervalMin;
}

function onclose(e) {
    if (!connectionOpened) {
        console.log('Failed to connect to the server.');
    }
    var canceled = ws == null;
    connectionOpened = false;
    ws = null;
    if (canceled) {
        // if connection is canceled.
        servers[0].status = 'disconnected';
        for (var i = 0; i < configPorts.length; ++i) {
            configPorts[i].postMessage(
                {'update': [servers[0].getStatusMessage()]});
        }
        return;
    }

    retryTimerId = setTimeout(connectToServer, retryInterval);
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
