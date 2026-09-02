// script for testing json rpc
// requires installing ws through package manager
const WebSocket = require("ws");
const ws = new WebSocket("ws://localhost:8080/ws");

if (process.argv.length < 4) {
    console.log("usage: node jrpc.js <method> <json-payload>");
    process.exit();
}

const method = process.argv[2];
const payload = process.argv[3];

ws.addEventListener("open", () => {
    console.log("connected");

    try {
        const params = JSON.parse(payload);
        ws.send(
            JSON.stringify({
                method,
                params,
            });
    } catch (e) {
        console.error(e)
    }
});

ws.addEventListener("message", (event) => {
    console.log("received:", event.data);
});

ws.addEventListener("error", (err) => {
    console.error("websocket error:", err);
});

ws.addEventListener("close", (event) => {
    console.log("closed:", event.code, event.reason);
});
