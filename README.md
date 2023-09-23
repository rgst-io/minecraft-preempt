# minecraft-preempt

A lightweight Minecraft server manager. Starts a server when users join, and stops them when they leave.

## Supported Clouds

- `gcp`
- `docker`

## Usage

First, define a configuration file for your server. The format is like so:

### Top level

| Key             | Description               |
| --------------- | ------------------------- |
| `listenAddress` | The address to listen on. |
| `servers`       | Array of all servers      |

#### Server

| Key             | Description                      |
| --------------- | -------------------------------- |
| `hostname`      | The hostname of the server.      |
| `listenAddress` | The address to listen on.        |
| `gcp`           | The GCP configuration            |
| `docker`        | The Docker configuration         |
| `whitelist`     | List of users allowed to connect |

### Cloud Configurations

#### GCP

| Key        | Description           |
| ---------- | --------------------- |
| `project`  | The GCP project ID    |
| `zone`     | The GCP zone          |
| `instance` | The GCP instance name |

#### Docker

| Key           | Description          |
| ------------- | -------------------- |
| `containerID` | Container ID or name |

Specifying a configuration can be done with `--config`, for a file path.
Or, for serverless environments, the config can be specified with the
`CONFIG` environment variable.

## License

GPL-3.0
