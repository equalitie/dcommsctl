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

This is far from complete. Look at `rpc_handlers.go` for arguments and return values

# Toools to add

* CreepPad
