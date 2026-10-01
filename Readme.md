# What this is

This is a back end for dComms DIY project. It creates instances of applications and runs these via `docker compose` suite. There's a special application `dawa` which is the WebUI to manage this back end. Other supported apps for now are Mastodon and Peertube.

NOTE: This is definitely not production quality but mostly a result of explorative programming sessions.

# How to run

Requirements
 * Go 

Run the back end

```
go run ./
```

# API

Connect to a websocket on port 6006 and call these actions. Some examples (though incomplete) are in `jrpc.html`

There is a `schema_jrpc.json` file in the root folder. It is generated every server start.

This is far from complete. Look at the `rpc_*.go` files for arguments and return values.

# Helper scripts

`cleanup.sh` - stops all services running from `.instances` folder and removes the folder after

# Logging

Logs go to stdout as JSON, one object per line. `LOG_LEVEL` sets the minimum severity
that is written; everything below it is dropped. Unset or unrecognised values fall back
to `info`. `none` silences everything.

| Level | Writes |
| --- | --- |
| `none` | nothing |
| `error` | failures: unreadable config, closed locks, failed jobs, bind and connect errors |
| `warn` | TBD |
| `info` | startup, the bound address, client connections, shutdown |
| `debug` | API requests and their responses, container inspection, caddy replies |

For example:

```
LOG_LEVEL=debug go run ./
```

RPC request and response bodies appear under a `json` key. Requests that carry no params
log no `json` key at all, which distinguishes them from an explicit `{}`.
