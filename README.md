# `rtcd`

`rtcd` (**r**eal-**t**ime **c**ommunication **d**aemon) is a service built to offload WebRTC and media processing tasks from [Mattermost Calls](https://github.com/mattermost/mattermost-plugin-calls) in order to efficiently support scalable and secure deployments of the plugin on both on-prem and cloud environments.

## Running

`make go-run`

## Testing

`make test`

## Configuration

Configuration is documented in-place through the [`config.sample.toml`](config/config.sample.toml) file.

## Documentation

Documentation and implementation details can be found in the [`docs`](docs/) folder.

## Get involved

Please join the [Project: rtcd](https://community.mattermost.com/core/channels/project-rtcd) channel to discuss any topic related to this project.

## License

See [LICENSE.txt](LICENSE.txt) for license rights and limitations.

