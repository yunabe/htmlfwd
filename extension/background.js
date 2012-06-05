

var retryIntervalMin = 10 * 1000;  // ms
var retryIntervalMax = 10 * 60 * 1000;

function addMessage(message) {
    console.log(message);
}

var Server = function(label, host) {
    this.label = label;
    this.host = host;
    this.status = 'connecting';
    this.nextRetryTime = 0;
    this.webSocket = null;
    this.retryTimerId = null;
    this.connectionOpened = false;
    this.retryInterval = retryIntervalMin;
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

Server.prototype.sendStatusUpdate = function() {
    var updates = [];
    for (var i = 0; i < servers.length; ++i) {
        if (servers[i] == this) {
            updates.push(this.getStatusMessage());
        } else {
            updates.push(null);
        }
    }
    for (var i = 0; i < configPorts.length; ++i) {
        configPorts[i].postMessage({'update': updates});
    }
}

Server.prototype.connectToServer = function() {
    // clear existing timer to avoid we create multi-timers.
    clearTimeout(this.retryTimerId);
    this.retryTimerId = null;
    if (this.webSocket) {
        return;
    }
    addMessage('connectToServer');
    var url = 'ws://' + this.host + '/ws';
    try {
        this.webSocket = new WebSocket(url);
    } catch (e) {
        this.webSocket = null;
    }
    if (this.webSocket) {
        this.webSocket.onmessage = this.onMessage.bind(this);
        this.webSocket.onopen = this.onOpen.bind(this);
        this.webSocket.onclose = this.onClose.bind(this);
        this.webSocket.onerror = this.onError.bind(this);
    } else {
        // TODO: show message to config panel.
        addMessage('Failed to create WebSocket. (url is bad?)');
    }
};

Server.prototype.disconnectFromServer = function() {
    if (!this.webSocket) {
        clearTimeout(this.retryTimerId);
        this.retryInterval = retryIntervalMin;
        this.status = 'disconnected';
        this.sendStatusUpdate();
        return;
    }
    this.webSocket.close();
    this.webSocket = null;
};

Server.prototype.onError = function(e) {
    addMessage('Encountered error.');
    addMessage(e)
};

Server.prototype.onOpen = function() {
    this.status = 'connected';
    this.sendStatusUpdate();
    this.connectionOpened = true;
    this.retryInterval = retryIntervalMin;
};

Server.prototype.onClose = function() {
    if (!this.connectionOpened) {
        console.log('Failed to connect to the server.');
    }
    var canceled = this.webSocket == null;
    this.connectionOpened = false;
    this.webSocket = null;
    if (canceled) {
        // if connection is canceled.
        this.status = 'disconnected';
        this.sendStatusUpdate();
        return;
    }

    this.retryTimerId = setTimeout(connectToServer, this.retryInterval);
    this.status = 'connecting';
    this.nextRetryTime = Date.now() + this.retryInterval;
    this.sendStatusUpdate();
    this.retryInterval = this.retryInterval * 2;
    if (this.retryInterval > retryIntervalMax) {
        this.retryInterval = retryIntervalMax;
    }
};

Server.prototype.onMessage = function(e) {
    addMessage('Recieved: ' + e.data);
    var obj = JSON.parse(e.data);
    var id = obj['Id']
    addMessage('Id: ' + id)
    var path = obj['OpenUrl']
    addMessage('url: ' + path)
    if (path) {
        var url = 'http://' + this.host + '/fwd/' + id + path;
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
};

var configPorts = [];

function onConnectConfig(port) {
    configPorts.push(port);
    var reloadData = [];
    for (var i = 0; i < servers.length; ++i) {
        reloadData.push(servers[i].getMessage());
    }
    port.postMessage({'reload': reloadData});
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
        var index = msg['index'];
        if (index < servers.length) {
            servers[index].connectToServer();
        } else {
            // This can happens if timing is bad.
            addMessage('ignore message because index is out of bound.');
        }
    } else if (msg['disconnect']) {
        var index = msg['index'];
        if (index < servers.length) {
            servers[index].disconnectFromServer();
        } else {
            addMessage('ignore message because index is out of bound.');
        }
    } else if (msg['reload']) {
        var settings = msg['reload'];
        console.log(settings);
        localStorage['server-settings'] = JSON.stringify(settings);
        // onclose fired by this disconnect is called 'after' connectToServer.
        // TODO: Fix this issue
        disconnectFromServer();
        setUpServers(settings);
        connectToServer();
    }
}

var servers = [];

function setUpServers(settings) {
    servers = [];
    reloadData = [];
    for (var i = 0; i < settings.length; ++i) {
        servers.push(new Server(
            settings[i]['label'], settings[i]['host'], 'connecting'));
        reloadData.push(servers[i].getMessage());
    }
    for (var i = 0; i < configPorts.length; ++i) {
        configPorts[i].postMessage({'reload': reloadData});
    }
}

function connectToServer() {
    for (var i = 0; i < servers.length; ++i) {
        servers[i].connectToServer();
    }
}

function disconnectFromServer() {
    for (var i = 0; i < servers.length; ++i) {
        servers[i].disconnectFromServer();
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
