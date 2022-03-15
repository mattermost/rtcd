# Project Structure

## [cmd/rtcd](../cmd/rtcd)

Main entry point for running the service. Implementation for the `rtcd` command lives here.

## [config](../config)

This folder contains configuration files (with samples).

## [logger](../logger)

This is where the logger implementation lives.

## [service](../service)

This is where the main service implementation lives.

### [service/api](../service/api)

This is where the HTTP API server implementation lives.

### [service/ws](../service/ws)

This is where the WebSocket server and client implementations live.

### [service/store](../service/store)

This is where the persistent data store implementation lives.

### [service/auth](../service/auth)

This is where the Authentication service implementation lives.
