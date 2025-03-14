# Running calls through `rtcd`

## Prerequisites

- An [Enterprise](https://docs.mattermost.com/about/editions-and-offerings.html#mattermost-enterprise) licensed Mattermost installation with [Calls](https://github.com/mattermost/mattermost-plugin-calls) installed.
- A working version of [Docker](https://www.docker.com/) installed, if [running as a container](#as-a-docker-container)
- [systemd](https://systemd.io/), if [running as a simple service](#as-a-systemd-service)

## Installation

As first step we fetch the latest official `rtcd` docker image:

```sh
docker pull mattermost/rtcd:latest
```

## Running

### As a Docker container

To start `rtcd` we can run the following command:

```sh
docker run --name rtcd -e "RTCD_LOGGER_ENABLEFILE=false" -e "RTCD_API_SECURITY_ALLOWSELFREGISTRATION=true" -p 8443:8443/udp -p 8443:8443/tcp -p 8045:8045/tcp mattermost/rtcd
```

> **_Note:_**
>
>- By default the service starts even if no configuration file is provided. In such case default values are used. In the example above we are overriding a couple of config settings:
>   - `logger.enable_file` We set this to `false` to prevent warnings since the process has no permissions to write files. If a log file is needed a volume should be mounted accordingly.
>   - `api.security.allow_self_registration` We set this to `true` so that clients (Mattermost instances) can automatically self register and authenticate to the service without manually having to create accounts. This is fine as long as the service is running in an internal/private network.
>- We are exposing to the host the two ports the service is listening on:
>   - `8443/udp` and `8443/tcp` are the ports used to route all the calls related media traffic (i.e. voice, screen share). The first one (UDP) is preferred but the latter (TCP) can be used as a fallback.
>   - `8045/tcp` Is the port used to serve the HTTP/WebSocket internal API to communicate with the Mattermost side (Calls plugin).

#### Running with config file

Of course it's also possible to provide the service with a complete config file by mounting a volume, e.g.:

```sh
docker run --name rtcd -v /path/to/rtcd/config:/config mattermost/rtcd -config /config/config.toml
```

### As a systemd service

Alternatively, the `rtcd` binary can be executed using a systemd service file.

```
sudo touch /lib/systemd/system/rtcd.service
```

```
[Unit]
Description=rtcd
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/rtcd
Restart=always
RestartSec=10
User=mattermost
Group=mattermost
Environment=API_SECURITY_ALLOWSELFREGISTRATION=true

[Install]
WantedBy=multi-user.target
```

> **_Note:_** By default the service starts even if no configuration file is provided. In such case default values are used. In the service file above we are overriding a config setting through environment variables:
>
> - `api.security.allow_self_registration` We set this to `true` so that clients (Mattermost instances) can automatically self register and authenticate to the service without manually having to create accounts. This is fine as long as the service is running in an internal/private network.

Load the service file:

```
sudo systemctl daemon-reload
```

Enable and start the service:

```
sudo systemctl enable --now /lib/systemd/system/rtcd.service
```

### Verify service is running

Finally, to verify that the service is correctly running we can try calling the HTTP API:

```sh
curl http://localhost:8045/version
```

This should return a JSON object with basic information about the service such as its build version.

## Configuration

Configuration for the service is fully documented in-place through the [`config.sample.toml`](../config/config.sample.toml) file.

## Running calls

The last step to get calls working through `rtcd` is to configure the Calls side to use the service. This is done via the **Admin Console -> Plugins -> Calls -> RTCD service URL** setting, which in this example will be set to `http://localhost:8045`.

> **_Note:_**
>
> 1. The client will self-register the first time it connects to the service and store the authentication key in the database. If no client ID is explicitly provided, the diagnostic ID of the instance will be used.
> 2. The RTCD service URL supports credentials in the form `http://clientID:authKey@hostname`. Alternitevly these can be passed through environment overrides, namely `MM_CALLS_RTCD_CLIENT_ID` and `MM_CALLS_RTCD_AUTH_KEY`.
