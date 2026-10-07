// script for testing json rpc
// requires installing ws through package manager
const WebSocket = require("ws");
const url = "ws://localhost:6006/ws";
const ws = new WebSocket(url);

if (process.argv.length < 4) {
    console.log("usage: node jrpc.js <method> <json-payload>");
    process.exit();
}

const method = process.argv[2];
const payload = process.argv[3];

ws.addEventListener("open", () => {
    console.log("connected to ", url, " sending...");

    const params = JSON.parse(payload);
    ws.send(
        JSON.stringify({
            jsonrpc: '2.0',
            method,
            params,
            id: 1,
        }));
});

ws.addEventListener("message", (event) => {
    console.log("received:", event.data);
    ws.close();
});

ws.addEventListener("error", (err) => {
    console.error("websocket error:", err.error.message);
    ws.close();
});

ws.addEventListener("close", (event) => {
    console.log("closed:", event.code, event.reason);
});
