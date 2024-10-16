# Traefik Umami Feeder

This plugin for enables [Traefik Reverse Proxy](https://traefik.io/traefik/) to feed [Umami Analytics](https://umami.is)
with tracking events.

It was created as an alternative to [traefik-umami-plugin](https://github.com/1cedsoda/traefik-umami-plugin) and
inspired by idea of [Plausible Feeder Traefik Plugin](https://github.com/safing/plausiblefeeder).

## Features

- [X] Super easy to setup, one middleware for all websites
- [X] Server Side Tracking, no need to add JavaScript to your websites
- [X] Fast and private analytics

## Installation

To [add this plugin to traefik](https://plugins.traefik.io/install) reference this repository as a plugin in the static
config.
The version references a git tag.

```yaml
experimental:
  plugins:
    umami-feeder:
      moduleName: github.com/astappiev/traefik-umami-feeder
      version: v1.0.0 # replace with latest version available
```

```toml
[experimental.plugins.umami-feeder]
  moduleName = "github.com/astappiev/traefik-umami-feeder"
  version = "v1.0.0" # replace with latest version available
```

With the plugin installed, you can configure a middleware in a dynamic configuration such as a `config.yml` or docker
labels.

```yaml
http:
  middlewares:
    my-umami-middleware:
      plugin:
        umami-feeder:
          umamiHost: "http://umami:3000"
          websites:
            "example.com": "d4617504-241c-4797-8eab-5939b367b3ad"
```

```toml
[http.middlewares]
  [http.middlewares.my-umami-middleware.plugin.umami-feeder]
    umamiHost = "umami:3000"

    [http.middlewares.my-umami-middleware.plugin.umami-feeder.websites]
      "example.com" = "d4617504-241c-4797-8eab-5939b367b3ad"
```

You have an option to give a list of domains to track (and their website IDs on Umami). \
Or, you can give a token and the list will be fetched from Umami. For this, you need
either [retrieve the token yourself](https://umami.is/docs/api/authentication), or use
username/password instead.

After that, you need to add the middleware to a [router](https://doc.traefik.io/traefik/routing/routers/#middlewares_1).
Remember to reference the
correct [provider namespace](https://doc.traefik.io/traefik/providers/overview/#provider-namespace).

E.g. as Docker labels:

```yaml
- "traefik.http.routers.whoami.middlewares=my-umami-middleware@file"
```

Or, for all routers in a static configuration:

```yaml
entryPoints:
  web:
    http:
      middlewares:
        - my-umami-middleware@file
```

## Configuration

| key                 | default | type       | description                                                                                                                                 |
|---------------------|---------|------------|---------------------------------------------------------------------------------------------------------------------------------------------|
| `umamiHost`         | -       | `string`   | Umami server url, reachable from within traefik (container), e.g. `http://umami:3000`                                                       |
| `umamiToken`        | -       | `string`   | An API Token, used to automatize work with websites, not needed if you provide `websites`                                                   |
| `umamiUsername`     | -       | `string`   | An alternative to `umamiToken`, you can provide an username and password                                                                    |
| `umamiPassword`     | -       | `string`   | Only in combination with `umamiUsername`                                                                                                    |
| `umamiTeamId`       | -       | `string`   | In order to organize websites, you can use Umami Teams                                                                                      |
| `websites`          | -       | `map`      | A map of hostnames and their associated Umami IDs. Can also be used to override or extend fetched websites                                  |
| `createNewWebsites` | false   | `bool`     | If set to `true`, will try to create a new website on Umami, if domain not found there                                                      |
| `trackAllResources` | false   | `bool`     | Defines whether all requests for any resource should be tracked. By default, only requests that are believed to contain content are tracked |
| `trackExtensions`   |         | `string[]` | Defines an alternative list of file extensions that should be tracked                                                                       |
| `debug`             | false   | `bool`     | Something doesn't work? Set to `true` to see more logs (plugins doesn't have access to Traefik's log level)                                 |
