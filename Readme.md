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

```
inst/install - create new instance of an application (docker compose project for now)
inst/start - start instance by instanceID
inst/stop - stop instance by instanceID
inst/backup - backup instance by instanceID
inst/restore - restore instance by instanceID
```

This is far from complete. Look at `rpc_handlers.go` for arguments and return values

# Toools to add

* CreepPad
