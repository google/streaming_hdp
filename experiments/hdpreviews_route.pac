function FindProxyForURL(url, host) {
    if (shExpMatch(url, "wss:*") || shExpMatch(url, "*127.0.0.1*") || shExpMatch(url, "*35.188.237.140:8449\/\*")) {
        return "DIRECT";
    } else {
        return "HTTPS 35.188.237.140:8444";
    }
}
