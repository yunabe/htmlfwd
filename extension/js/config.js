// This file was automatically generated from config.soy.
// Please don't edit this file by hand.

goog.provide('yunabe.htmlfwd.config');

goog.require('yunabe.htmlfwd.soy');

goog.require('goog.events');

/**
 * @constructor
 */
yunabe.htmlfwd.config.ServerEntry = function(label, host, status, opt_retry_sec) {
    this.label = label;
    this.host = host;
    this.status = status;
    this.bodyId = 'body' + goog.getUid(this);
    this.checkId = 'check' + goog.getUid(this);
    this.labelId = 'label' + goog.getUid(this);
    this.retry_sec = opt_retry_sec || 0;
};

yunabe.htmlfwd.config.ServerEntry.prototype.render = function() {
    setTimeout(goog.bind(this.registerCheckboxCallback, this), 0);
    return yunabe.htmlfwd.soy.serverEntry(
        {label: this.label, host: this.host,
         status: this.status, retry_sec: this.retry_sec,
         bodyId: this.bodyId, checkId: this.checkId, labelId: this.labelId});
};

yunabe.htmlfwd.config.ServerEntry.prototype.updateStatus = function(status, opt_retry_sec) {
    this.status = status;
    this.retry_sec = opt_retry_sec || 0;
    document.getElementById(this.bodyId).className = 'status ' + this.status;
    document.getElementById(this.labelId).innerHTML =
        yunabe.htmlfwd.soy.statusLabel({status: this.status,
                                        retry_sec: this.retry_sec});
};

yunabe.htmlfwd.config.ServerEntry.prototype.decrementRetrySec = function() {
    if (this.status != 'connecting' || this.retry_sec <= 0) {
        return false;
    }
    this.retry_sec -= 1;
    document.getElementById(this.labelId).innerHTML =
        yunabe.htmlfwd.soy.statusLabel({status: this.status,
                                        retry_sec: this.retry_sec});
    if (this.retry_sec <= 0) {
        return false;
    } else {
        return true;
    }
};

yunabe.htmlfwd.config.ServerEntry.prototype.connectToServer = function() {
    // TODO
};

yunabe.htmlfwd.config.ServerEntry.prototype.disconnectFromServer = function() {
    // TODO
};

yunabe.htmlfwd.config.ServerEntry.prototype.registerCheckboxCallback = function() {
    goog.events.listen(document.getElementById(this.checkId), 'click',
                       goog.bind(this.onCheckboxClicked, this));
};

yunabe.htmlfwd.config.ServerEntry.prototype.onCheckboxClicked = function() {
    var checked = document.getElementById(this.checkId).checked;
    if (checked) {
        this.connectToServer();
    } else {
        this.disconnectFromServer();
    }
};

var serverEntries = [];

var runCountDown = false;

var countDownRetrySec = function() {
    runCountDown = false;
    for (var i = 0; i < serverEntries.length; ++i) {
        if (serverEntries[i].decrementRetrySec()) {
            runCountDown = true;
        }
    }
    if (runCountDown) {
        setTimeout(countDownRetrySec, 1000);
    }
};

var startCountDownRetrySec = function() {
    if (!runCountDown) {
        runCountDown = true;
        setTimeout(countDownRetrySec, 1000);
    }
};

var onMessageFromBackground = function(msg, port) {
    if (msg['reload']) {
        serverEntries = [];
        var servers = msg['reload'];
        var htmls = [];
        for (var i = 0; i < servers.length; ++i) {
            var setting = servers[i];
            var server = new yunabe.htmlfwd.config.ServerEntry(
                setting['label'], setting['host'], setting['status'],
                setting['retry_sec'] || 0);
            serverEntries.push(server);
            htmls.push(server.render());
        }
        document.getElementById('server-list').innerHTML = htmls.join('');
        startCountDownRetrySec();
    }
    if (msg['update']) {
        var updates = msg['update'];
        for (var i = 0;
             i < serverEntries.length && i < updates.length;
             ++i) {
            var update = updates[i];
            if (!update) {
                continue;
            }
            serverEntries[i].updateStatus(update['status'],
                                          update['retry_sec']);
        }
        startCountDownRetrySec();
    }
};

var main = function() {
    var mainBody = document.getElementById('main');
    setTimeout(function() {
        mainBody.style['-webkit-transform'] = 'translate(-300px)';
    }, 1000);
    setTimeout(function() {
        mainBody.style['-webkit-transform'] = 'translate(0px)';
    }, 2000);

    var port = chrome.extension.connect();
    port['onMessage']['addListener'](onMessageFromBackground);
};

goog.events.listen(window, 'load', main);
