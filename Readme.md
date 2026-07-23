# What this is

This is definitely not production quality but mostly a result of explorative programming session.

# How to run

Requirements
 * Podman
 * Go 
 * A web browser

Run podman as a process:

```
podman --log-level=debug system service -t0
```

Run the backend

```
chmod +x ./run.sh
./run.sh
```

Open url `http://localhost:8080/` in any browser

# What is implemented

You can run two hard-coded services (mastodon and peertube), you can get environment variables from peertube. Mastodon fails to do that due to one of the containers, I did not have time to debug that.

Panic button for now only acts as a "compose down both services".

You can open an environment file for each service for edit and save it (and then you have to manually restart the service).

The page does not check the initial state of services so they'll always be in "off" state after reload. But it subscribes to changes in this state, so the state (red or green light) changes whenever backend turns the service on or off.
